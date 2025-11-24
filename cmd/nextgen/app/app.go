// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	goruntime "runtime"
	"strconv"
	"time"

	cmdutils "github.com/gardener/gardener/cmd/utils/initrun"
	"github.com/gardener/gardener/extensions/pkg/controller/cmd"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	"github.com/gardener/gardener/pkg/controllerutils/routes"
	gardenerhealthz "github.com/gardener/gardener/pkg/healthz"
	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/component-base/version/verflag"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	controllerconfig "sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/yaml"

	dnsmanv1alpha1 "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	configv1alpha1 "github.com/gardener/external-dns-management/pkg/dnsman2/apis/config/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/app"
	"github.com/gardener/external-dns-management/pkg/dnsman2/app/appcontext"
	dnsmanclient "github.com/gardener/external-dns-management/pkg/dnsman2/client"
)

// Name is the name of the dns-controller-manager.
const Name = "dns-controller-manager-next-generation"

var configDecoder runtime.Decoder

func init() {
	configScheme := runtime.NewScheme()
	schemeBuilder := runtime.NewSchemeBuilder(
		config.AddToScheme,
		configv1alpha1.AddToScheme,
		dnsmanv1alpha1.AddToScheme,
	)
	utilruntime.Must(schemeBuilder.AddToScheme(configScheme))
	configDecoder = serializer.NewCodecFactory(configScheme).UniversalDecoder()
}

// NewCommand returns a new controller-manager command.
func NewCommand() *cobra.Command {
	o := newOptions()
	cmd := &cobra.Command{
		Use:   Name,
		Short: "Launch the " + Name,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			log, err := cmdutils.InitRun(cmd, o, Name)
			if err != nil {
				return err
			}

			if err := o.run(cmd.Context(), log); err != nil {
				log.Error(err, "Launching "+Name+" failed")
			}
			return nil
		},
	}

	flags := cmd.Flags()
	o.addFlags(flags)
	verflag.AddFlags(flags)

	return cmd
}

// options is a struct to support packages command.
type options struct {
	configFile         string
	verbose            bool
	config             *config.DNSManagerConfiguration
	controllerSwitches *cmd.SwitchOptions
}

// newOptions returns initialized options.
func newOptions() *options {
	return &options{
		controllerSwitches: app.ControllerSwitches(),
	}
}

// addFlags binds the command options to a given flagset.
func (o *options) addFlags(flags *pflag.FlagSet) {
	flags.StringVar(&o.configFile, "config", o.configFile, "Path to configuration file.")
	flags.BoolVar(&o.verbose, "v", o.verbose, "If true, overwrites log level in config with value 'debug'.")
	o.controllerSwitches.AddFlags(flags)
}

// Complete adapts from the command line args to the data required.
func (o *options) Complete() error {
	if len(o.configFile) == 0 {
		return fmt.Errorf("missing config file")
	}

	data, err := os.ReadFile(o.configFile)
	if err != nil {
		return fmt.Errorf("error reading config file: %w", err)
	}

	o.config = &config.DNSManagerConfiguration{}
	if err = runtime.DecodeInto(configDecoder, data, o.config); err != nil {
		return fmt.Errorf("error decoding config: %w", err)
	}

	if err := o.controllerSwitches.Complete(); err != nil {
		return fmt.Errorf("error completing controller switches: %w", err)
	}

	return nil
}

// Validate validates the provided command options.
func (o *options) Validate() error {
	return nil
}

// LogConfig returns the logging config.
func (o *options) LogConfig() (logLevel, logFormat string) {
	logLevel = o.config.LogLevel
	logFormat = o.config.LogFormat
	if o.verbose {
		logLevel = "debug"
	}
	return
}

// run does the actual work of the command.
func (o *options) run(ctx context.Context, log logr.Logger) error {
	cfg := o.config

	if err := logConfiguration(log, cfg); err != nil {
		return fmt.Errorf("error logging configuration: %w", err)
	}

	log.Info("Getting rest config")
	if cfg.ClientConnection.Kubeconfig == "" {
		if kubeconfig := os.Getenv("KUBECONFIG"); kubeconfig != "" {
			log.Info("Using kubeconfig from environment variable KUBECONFIG", "KUBECONFIG", kubeconfig)
			cfg.ClientConnection.Kubeconfig = kubeconfig
		} else {
			log.Info("No kubeconfig specified, assuming in-cluster configuration")
		}
	}

	restConfig, err := kubernetes.RESTConfigFromClientConnectionConfiguration(&cfg.ClientConnection.ClientConnectionConfiguration, nil, kubernetes.AuthTokenFile)
	if err != nil {
		return err
	}
	var controlPlaneRestConfig *rest.Config
	switch cfg.ControlPlaneClientConnection.Kubeconfig {
	case "":
		log.Info("Using the main kubeconfig for the control plane as well")
		controlPlaneRestConfig = restConfig
	case "IN-CLUSTER":
		log.Info("Using in-cluster configuration for control plane")
		controlPlaneRestConfig, err = rest.InClusterConfig()
		if err != nil {
			return err
		}
	default:
		log.Info("Using control plane kubeconfig", "kubeconfig", cfg.ControlPlaneClientConnection.Kubeconfig)
		controlPlaneRestConfig, err = kubernetes.RESTConfigFromClientConnectionConfiguration(&cfg.ControlPlaneClientConnection.ClientConnectionConfiguration, nil, kubernetes.AuthTokenFile)
		if err != nil {
			return err
		}
	}

	var extraHandlers map[string]http.Handler
	if cfg.Debugging != nil && cfg.Debugging.EnableProfiling {
		extraHandlers = routes.ProfilingHandlers
		if cfg.Debugging.EnableContentionProfiling {
			goruntime.SetBlockProfileRate(1)
		}
	}

	log.Info("Setting up manager")
	managerOptions := manager.Options{
		Logger:                  log,
		Scheme:                  dnsmanclient.ClusterScheme,
		GracefulShutdownTimeout: ptr.To(5 * time.Second),
		Cache: cache.Options{
			SyncPeriod:        &cfg.ClientConnection.CacheResyncPeriod.Duration,
			DefaultNamespaces: map[string]cache.Config{cfg.Controllers.DNSProvider.Namespace: {}},

			/*
				ByObject: map[client.Object]cache.ByObject{
					&corev1.Secret{}: {
						Transform: func(i interface{}) (interface{}, error) {
							return corev1.Secret{
								ObjectMeta: i.(*corev1.Secret).ObjectMeta,
								Type:       i.(*corev1.Secret).Type,
								Immutable:  i.(*corev1.Secret).Immutable,
							}, nil
						},
					},
				},
			*/
		},

		HealthProbeBindAddress: net.JoinHostPort(cfg.Server.HealthProbes.BindAddress, strconv.Itoa(cfg.Server.HealthProbes.Port)),
		Metrics: metricsserver.Options{
			BindAddress:   net.JoinHostPort(cfg.Server.Metrics.BindAddress, strconv.Itoa(cfg.Server.Metrics.Port)),
			ExtraHandlers: extraHandlers,
		},

		LeaderElection:                cfg.LeaderElection.LeaderElect,
		LeaderElectionResourceLock:    cfg.LeaderElection.ResourceLock,
		LeaderElectionID:              cfg.LeaderElection.ResourceName,
		LeaderElectionNamespace:       cfg.LeaderElection.ResourceNamespace,
		LeaderElectionReleaseOnCancel: true,
		LeaseDuration:                 &cfg.LeaderElection.LeaseDuration.Duration,
		RenewDeadline:                 &cfg.LeaderElection.RenewDeadline.Duration,
		RetryPeriod:                   &cfg.LeaderElection.RetryPeriod.Duration,
		Controller: controllerconfig.Controller{
			RecoverPanic: ptr.To(true),
		},
	}
	if controlPlaneRestConfig != restConfig {
		managerOptions.Cache.DefaultNamespaces = nil // all namespaces
	}
	mgr, err := manager.New(restConfig, managerOptions)
	if err != nil {
		return err
	}
	var controlPlaneCluster cluster.Cluster = mgr
	if controlPlaneRestConfig != restConfig {
		log.Info("Setting up cluster object for target")
		controlPlaneCluster, err = cluster.New(controlPlaneRestConfig, func(opts *cluster.Options) {
			opts.Scheme = dnsmanclient.ClusterScheme
			opts.Logger = log

			// use dynamic rest mapper for secondary cluster, which will automatically rediscover resources on NoMatchErrors
			// but is rate-limited to not issue to many discovery calls (rate-limit shared across all reconciliations)
			opts.MapperProvider = apiutil.NewDynamicRESTMapper

			opts.Cache.DefaultNamespaces = map[string]cache.Config{cfg.Controllers.DNSProvider.Namespace: {}}
			opts.Cache.SyncPeriod = &cfg.ControlPlaneClientConnection.CacheResyncPeriod.Duration

			opts.Client.Cache = &client.CacheOptions{
				DisableFor: []client.Object{
					&corev1.Event{},
				},
			}
		})
		if err != nil {
			return fmt.Errorf("could not instantiate control plane cluster: %w", err)
		}

		log.Info("Setting up ready check for control plane informer sync")
		if err := mgr.AddReadyzCheck("control-plane-informer-sync", gardenerhealthz.NewCacheSyncHealthz(controlPlaneCluster.GetCache())); err != nil {
			return err
		}

		log.Info("Adding control plane cluster to manager")
		if err := mgr.Add(controlPlaneCluster); err != nil {
			return fmt.Errorf("failed adding control plane cluster to manager: %w", err)
		}
	}

	if err := mgr.AddHealthzCheck("ping", healthz.Ping); err != nil {
		return err
	}
	if err := mgr.AddReadyzCheck("informer-sync", gardenerhealthz.NewCacheSyncHealthz(mgr.GetCache())); err != nil {
		return err
	}

	log.Info("Adding field indexes to informers")
	if err := app.AddAllFieldIndexesToCluster(ctx, controlPlaneCluster); err != nil {
		return fmt.Errorf("failed adding indexes: %w", err)
	}

	if err := app.DeployCRDs(ctx, log, mgr.GetConfig(), cfg); err != nil {
		return fmt.Errorf("failed deploying CRDs: %w", err)
	}

	addCtx := appcontext.NewAppContext(ctx, log, controlPlaneCluster, cfg)
	if err := o.controllerSwitches.Completed().AddToManager(addCtx, mgr); err != nil {
		return fmt.Errorf("failed adding controllers to manager: %w", err)
	}

	log.Info("Starting manager")
	return mgr.Start(ctx)
}

func logConfiguration(log logr.Logger, cfg *config.DNSManagerConfiguration) error {
	cfg1 := &configv1alpha1.DNSManagerConfiguration{}
	if err := configv1alpha1.Convert_config_DNSManagerConfiguration_To_v1alpha1_DNSManagerConfiguration(cfg, cfg1, nil); err != nil {
		return fmt.Errorf("error converting config to v1alpha1 for logging: %w", err)
	}

	data, err := yaml.Marshal(cfg1)
	if err != nil {
		return fmt.Errorf("error marshalling config to yaml for logging: %w", err)
	}

	log.Info("Using configuration:\n" + string(data))
	return nil
}
