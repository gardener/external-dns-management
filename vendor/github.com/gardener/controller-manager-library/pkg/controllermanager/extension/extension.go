/*
 * SPDX-FileCopyrightText: 2020 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package extension

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/gardener/controller-manager-library/pkg/config"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/cluster"
	"github.com/gardener/controller-manager-library/pkg/ctxutil"
	"github.com/gardener/controller-manager-library/pkg/resources"

	areacfg "github.com/gardener/controller-manager-library/pkg/controllermanager/config"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/utils"
)

type ExtensionDefinitions map[string]Definition
type ExtensionTypes map[string]ExtensionType
type Extensions map[string]Extension

type ExtensionType interface {
	Name() string
	Definition() Definition
}

type Definition interface {
	OrderedElem

	Names() utils.StringSet
	Size() int
	Description() string
	Validate() error
	ExtendConfig(*areacfg.Config)
	CreateExtension(cm ControllerManager) (Extension, error)
}

type Extension interface {
	Name() string
	//Definition() Definition
	RequiredClusters() (clusters utils.StringSet, err error)
	RequiredClusterIds(clusters cluster.Clusters) utils.StringSet
	Setup(ctx context.Context) error
	Start(ctx context.Context) error
}

type ExtensionRegistry interface {
	RegisterExtension(e ExtensionType) error
	MustRegisterExtension(e ExtensionType)
	GetExtensionTypes() ExtensionTypes
	GetDefinitions() ExtensionDefinitions
}

type ExtensionDefinitionBase struct {
	name   string
	after  []string
	before []string
}

var _ OrderedElem = (*ExtensionDefinitionBase)(nil)

func NewExtensionDefinitionBase(name string, orders ...[]string) ExtensionDefinitionBase {
	orders = append(orders, nil, nil)
	return ExtensionDefinitionBase{
		name:   name,
		after:  orders[0],
		before: orders[1],
	}
}

func (this *ExtensionDefinitionBase) Name() string {
	return this.name
}

func (this *ExtensionDefinitionBase) After() []string {
	return this.after
}

func (this *ExtensionDefinitionBase) Before() []string {
	return this.before
}

////////////////////////////////////////////////////////////////////////////////

type _ExtensionRegistry struct {
	lock       sync.Mutex
	extensions ExtensionTypes
}

func NewExtensionRegistry() ExtensionRegistry {
	return &_ExtensionRegistry{extensions: ExtensionTypes{}}
}

func (this *_ExtensionRegistry) RegisterExtension(e ExtensionType) error {
	this.lock.Lock()
	defer this.lock.Unlock()
	if this.extensions[e.Name()] != nil {
		return fmt.Errorf("extension with name %q already registered", e.Name())
	}
	this.extensions[e.Name()] = e
	return nil
}

func (this *_ExtensionRegistry) MustRegisterExtension(e ExtensionType) {
	if err := this.RegisterExtension(e); err != nil {
		panic(err)
	}
}

func (this *_ExtensionRegistry) GetExtensionTypes() ExtensionTypes {
	this.lock.Lock()
	defer this.lock.Unlock()

	ext := ExtensionTypes{}
	for n, t := range this.extensions {
		ext[n] = t
	}
	return ext
}

func (this *_ExtensionRegistry) GetDefinitions() ExtensionDefinitions {
	this.lock.Lock()
	defer this.lock.Unlock()

	ext := ExtensionDefinitions{}
	for n, e := range this.extensions {
		ext[n] = e.Definition()
	}
	return ext
}

var extensions = NewExtensionRegistry()

func DefaultRegistry() ExtensionRegistry {
	return extensions
}

func RegisterExtension(e ExtensionType) {
	extensions.RegisterExtension(e)
}

////////////////////////////////////////////////////////////////////////////////

type MaintainerInfo = areacfg.MaintainerInfo

type ControllerManager interface {
	GetName() string
	GetMaintainer() MaintainerInfo
	GetNamespace() string

	GetConfig() *areacfg.Config
	GetDefaultScheme() *runtime.Scheme
	NewContext(key, value string) logger.LogContext
	GetContext() context.Context

	GetCluster(name string) cluster.Interface
	GetClusters() cluster.Clusters
	ClusterDefinitions() cluster.Definitions

	GetExtension(name string) Extension

	GetClusterIdMigration() resources.ClusterIdMigration
}

type Environment interface {
	logger.LogContext
	ControllerManager() ControllerManager
	Name() string
	Namespace() string
	GetContext() context.Context
	GetCluster(name string) cluster.Interface
	GetClusters() cluster.Clusters
	GetDefaultScheme() *runtime.Scheme
	ClusterDefinitions() cluster.Definitions
}

type environment struct {
	logger.LogContext
	name    string
	context context.Context
	manager ControllerManager
}

func NewDefaultEnvironment(ctx context.Context, name string, manager ControllerManager) Environment {
	if ctx == nil {
		ctx = manager.GetContext()
	}
	logctx := manager.NewContext("extension", name)
	return &environment{
		LogContext: logctx,
		name:       name,
		context:    logger.Set(ctx, logctx),
		manager:    manager,
	}
}

func (this *environment) ControllerManager() ControllerManager {
	return this.manager
}

func (this *environment) Name() string {
	return this.name
}

func (this *environment) Namespace() string {
	return this.manager.GetNamespace()
}

func (this *environment) GetContext() context.Context {
	return this.context
}

func (this *environment) GetCluster(name string) cluster.Interface {
	return this.manager.GetCluster(name)
}

func (this *environment) GetClusters() cluster.Clusters {
	return this.manager.GetClusters()
}

func (this *environment) GetDefaultScheme() *runtime.Scheme {
	return this.manager.GetDefaultScheme()
}

func (this *environment) ClusterDefinitions() cluster.Definitions {
	return this.manager.ClusterDefinitions()
}

////////////////////////////////////////////////////////////////////////////////

type ElementBase interface {
	logger.LogContext

	GetType() string
	GetName() string

	GetContext() context.Context

	GetOptionSource(name string) (config.OptionSource, error)
	GetOption(name string) (*config.ArbitraryOption, error)
	GetBoolOption(name string) (bool, error)
	GetStringOption(name string) (string, error)
	GetStringArrayOption(name string) ([]string, error)
	GetIntOption(name string) (int, error)
	GetDurationOption(name string) (time.Duration, error)
}

type elementBase struct {
	logger.LogContext
	name     string
	typeName string
	context  context.Context
	options  config.OptionGroup
}

func NewElementBase(ctx context.Context, valueType ctxutil.ValueKey, element interface{}, name string, set config.OptionGroup) ElementBase {
	ctx = valueType.WithValue(ctx, name)
	ctx, logctx := logger.WithLogger(ctx, valueType.Name(), name)
	return &elementBase{
		LogContext: logctx,
		context:    ctx,
		name:       name,
		typeName:   valueType.Name(),
		options:    set,
	}
}

func (this *elementBase) GetName() string {
	return this.name
}

func (this *elementBase) GetType() string {
	return this.typeName
}

func (this *elementBase) GetContext() context.Context {
	return this.context
}

func (this *elementBase) GetOption(name string) (*config.ArbitraryOption, error) {
	opt := this.options.GetOption(name)
	if opt == nil {
		return nil, fmt.Errorf("unknown option %q for %s %q", name, this.GetType(), this.GetName())
	}
	return opt, nil
}

func (this *elementBase) GetOptionSource(name string) (config.OptionSource, error) {
	src := this.options.GetSource(name)
	if src == nil {
		return nil, fmt.Errorf("unknown option source %q for %s %q", name, this.GetType(), this.GetName())
	}
	return src, nil
}

func (this *elementBase) GetBoolOption(name string) (bool, error) {
	opt, err := this.GetOption(name)
	if err != nil {
		return false, err
	}
	return opt.BoolValue(), nil
}

func (this *elementBase) GetStringOption(name string) (string, error) {
	opt, err := this.GetOption(name)
	if err != nil {
		return "", err
	}
	return opt.StringValue(), nil
}

func (this *elementBase) GetStringArrayOption(name string) ([]string, error) {
	opt, err := this.GetOption(name)
	if err != nil {
		return []string{}, err
	}
	return opt.StringArray(), nil
}

func (this *elementBase) GetIntOption(name string) (int, error) {
	opt, err := this.GetOption(name)
	if err != nil {
		return 0, err
	}
	return opt.IntValue(), nil
}

func (this *elementBase) GetDurationOption(name string) (time.Duration, error) {
	opt, err := this.GetOption(name)
	if err != nil {
		return 0, err
	}
	return opt.DurationValue(), nil
}

////////////////////////////////////////////////////////////////////////////////

type OptionDefinition interface {
	GetName() string
	Type() config.OptionType
	Default() interface{}
	Description() string
}

type OptionDefinitions map[string]OptionDefinition

////////////////////////////////////////////////////////////////////////////////

type OptionSourceCreator func() config.OptionSource

func OptionSourceCreatorByExample(proto config.OptionSource) OptionSourceCreator {
	if proto == nil {
		return nil
	}
	t := reflect.TypeOf(proto)
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return func() config.OptionSource {
		return reflect.New(t).Interface().(config.OptionSource)
	}
}

///////////////////////////////////////////////////////////////////////////////

type OptionSourceDefinition interface {
	GetName() string
	Create() config.OptionSource
}

type OptionSourceDefinitions map[string]OptionSourceDefinition

///////////////////////////////////////////////////////////////////////////////

type DefaultOptionDefinition struct {
	name         string
	gotype       config.OptionType
	defaultValue interface{}
	desc         string
}

func NewOptionDefinition(name string, gotype config.OptionType, def interface{}, desc string) OptionDefinition {
	return &DefaultOptionDefinition{name, gotype, def, desc}
}

func (this *DefaultOptionDefinition) GetName() string {
	return this.name
}

func (this *DefaultOptionDefinition) Type() config.OptionType {
	return this.gotype
}

func (this *DefaultOptionDefinition) Default() interface{} {
	return this.defaultValue
}

func (this *DefaultOptionDefinition) Description() string {
	return this.desc
}

var _ OptionDefinition = &DefaultOptionDefinition{}

///////////////////////////////////////////////////////////////////////////////

type DefaultOptionSourceSefinition struct {
	name    string
	creator OptionSourceCreator
}

func NewOptionSourceDefinition(name string, creator OptionSourceCreator) OptionSourceDefinition {
	return &DefaultOptionSourceSefinition{name, creator}
}

func (this *DefaultOptionSourceSefinition) GetName() string {
	return this.name
}

func (this *DefaultOptionSourceSefinition) Create() config.OptionSource {
	return this.creator()
}
