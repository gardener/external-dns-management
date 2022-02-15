/*
 * Copyright 2020 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *       http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 *
 */

package remoteaccesscertificates

import (
	"crypto/rsa"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"time"

	"github.com/gardener/controller-manager-library/pkg/config"
	"github.com/gardener/controller-manager-library/pkg/resources/apiextensions"
	"github.com/gardener/controller-manager-library/pkg/utils"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
	"go.uber.org/atomic"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"

	"github.com/gardener/external-dns-management/pkg/apis/dns/crds"
	api "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	dnsutils "github.com/gardener/external-dns-management/pkg/dns/utils"
)

const CONTROLLER = "remoteaccesscertificates"
const OPT_REMOTE_ACCESS_CAKEY = "remote-access-cakey"

func init() {
	crds.AddToRegistry(apiextensions.DefaultRegistry())

	controller.Configure(CONTROLLER).
		Reconciler(Create).
		DefaultWorkerPool(1, 0*time.Second).
		OptionsByExample("options", &Config{}).
		CustomResourceDefinitions(resources.NewGroupKind(api.GroupName, api.RemoteAccessCertificateKind)).
		MainResource(api.GroupName, api.RemoteAccessCertificateKind).
		ActivateExplicitly().
		MustRegister()
}

type Config struct {
	caKeyFile  string
	caCertFile string
}

func (r *Config) AddOptionsToSet(set config.OptionSet) {
	set.AddStringOption(&r.caKeyFile, OPT_REMOTE_ACCESS_CAKEY, "", "", "filename for private key of client CA")
	set.AddStringOption(&r.caCertFile, provider.OPT_REMOTE_ACCESS_CACERT, "", "", "filename for certificate of client CA")
}

func (r *Config) Evaluate() error {
	return nil
}

type reconciler struct {
	reconcile.DefaultReconciler
	controller         controller.Interface
	secretResources    resources.Interface
	certResources      resources.Interface
	config             *Config
	clientCACert       *x509.Certificate
	clientCAPrivateKey *rsa.PrivateKey

	nextSerialNumber atomic.Int64
}

var _ reconcile.Interface = &reconciler{}

///////////////////////////////////////////////////////////////////////////////

func Create(controller controller.Interface) (reconcile.Interface, error) {
	cfg, err := controller.GetOptionSource("options")
	config := cfg.(*Config)
	if err != nil {
		return nil, err
	}

	caCert, caPrivateKey, err := loadClientCA(config)
	if err != nil {
		return nil, err
	}

	secretResources, err := controller.GetMainCluster().Resources().GetByExample(&corev1.Secret{})
	if err != nil {
		return nil, err
	}
	certResources, err := controller.GetMainCluster().Resources().GetByExample(&api.RemoteAccessCertificate{})
	if err != nil {
		return nil, err
	}
	return &reconciler{
		controller:         controller,
		secretResources:    secretResources,
		certResources:      certResources,
		config:             config,
		clientCACert:       caCert,
		clientCAPrivateKey: caPrivateKey,
	}, nil
}

func loadClientCA(config *Config) (*x509.Certificate, *rsa.PrivateKey, error) {
	if config.caCertFile == "" {
		return nil, nil, fmt.Errorf("missing option %s with CA for client certificates", provider.OPT_REMOTE_ACCESS_CACERT)
	}
	pemClientCA, err := ioutil.ReadFile(config.caCertFile)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot read client CA: %w", err)
	}
	clientCA, err := DecodeCert(pemClientCA)
	if err != nil {
		return nil, nil, err
	}

	if config.caKeyFile == "" {
		return nil, nil, fmt.Errorf("missing option %s with CA key for client certificates", OPT_REMOTE_ACCESS_CAKEY)
	}
	keyPem, err := ioutil.ReadFile(config.caKeyFile)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot read client CA key: %w", err)
	}
	caPrivateKey, err := DecodePrivateKey(keyPem)
	if err != nil {
		return nil, nil, err
	}

	return clientCA, caPrivateKey, nil
}

func (r *reconciler) Setup() {
	r.nextSerialNumber.Store(time.Now().Unix())
	r.controller.Infof("### setup remote client certificate")
	res, _ := r.controller.GetMainCluster().Resources().GetByExample(&api.RemoteAccessCertificate{})
	list, _ := res.ListCached(labels.Everything())
	dnsutils.ProcessElements(list, func(e resources.Object) {
		r.reconcile(r.controller, e)
	}, 1)
}

///////////////////////////////////////////////////////////////////////////////

func (r *reconciler) Reconcile(logger logger.LogContext, obj resources.Object) reconcile.Status {
	err := r.reconcile(logger, obj)
	return reconcile.DelayOnError(logger, err)
}

func (r *reconciler) reconcile(logger logger.LogContext, obj resources.Object) error {
	cert := obj.Data().(*api.RemoteAccessCertificate)

	hasSecret := cert.Status.NotBefore != nil
	if hasSecret {
		_, err := r.getSecret(cert.Namespace, cert.Spec.SecretName)
		if err != nil {
			if !errors.IsNotFound(err) {
				return err
			}
			hasSecret = false
		}
	}

	if hasSecret && cert.Spec.Recreate {
		logger.Infof("prepare for recreating certificate secret")
		return r.resetForCertificatRecreation(cert)
	}

	if hasSecret && !cert.Status.Recreating {
		return nil
	}

	switch cert.Spec.Type {
	case api.ServerType:
		return r.createServerCertificate(cert)
	case api.ClientType:
		return r.createClientCertificate(cert)
	default:
		return fmt.Errorf("invalid .spec.type %s", cert.Spec.Type)
	}
}

func (r *reconciler) createServerCertificate(cert *api.RemoteAccessCertificate) error {
	subject := CreateSubject(cert.Spec.DomainName)

	cdata, err := CreateCertificate(r.clientCACert, r.clientCAPrivateKey, subject, cert.Spec.DomainName,
		cert.Spec.Days, r.nextSerialNumber.Inc(), true)
	if err != nil {
		return err
	}

	return r.writeSecretAndStatus(cert, cdata, nil)
}

func (r *reconciler) createClientCertificate(cert *api.RemoteAccessCertificate) error {
	commonName := cert.Namespace + "." + cert.Spec.DomainName
	subject := CreateSubject(commonName)

	cdata, err := CreateCertificate(r.clientCACert, r.clientCAPrivateKey, subject, cert.Spec.DomainName,
		cert.Spec.Days, r.nextSerialNumber.Inc(), false)
	if err != nil {
		return err
	}

	additionalData := map[string][]byte{
		"NAMESPACE": []byte(cert.Namespace),
	}

	return r.writeSecretAndStatus(cert, cdata, additionalData)
}

func (r *reconciler) writeSecretAndStatus(cert *api.RemoteAccessCertificate, cdata *CertData, additionalData map[string][]byte) error {
	secretData := map[string][]byte{
		corev1.TLSPrivateKeyKey: cdata.TLSKey,
		corev1.TLSCertKey:       cdata.TLSCrt,
		"ca.crt":                cdata.CACrt, // using same CA for client and server
		"NAMESPACE":             []byte(cert.Namespace),
	}
	for k, v := range additionalData {
		secretData[k] = v
	}

	secret := &corev1.Secret{}
	secret.Namespace = cert.Namespace
	secret.Name = cert.Spec.SecretName
	secret.Type = corev1.SecretTypeTLS
	secret.Data = secretData
	_, err := r.secretResources.CreateOrUpdate(secret)
	logger.Infof("created certificate secret %s/%s", secret.Namespace, secret.Name)
	if err != nil {
		_, _, err = r.certResources.ModifyStatus(cert, func(data resources.ObjectData) (bool, error) {
			o := data.(*api.RemoteAccessCertificate)
			mod := utils.ModificationState{}
			mod.AssureStringValue(&o.Status.Message, err.Error())
			return mod.IsModified(), nil
		})
		return err
	}

	_, _, err = r.certResources.ModifyStatus(cert, func(data resources.ObjectData) (bool, error) {
		o := data.(*api.RemoteAccessCertificate)
		mod := utils.ModificationState{}
		o.Status.NotAfter = &metav1.Time{Time: cdata.Certificate.NotAfter}
		o.Status.NotBefore = &metav1.Time{Time: cdata.Certificate.NotBefore}
		sn := cdata.Certificate.SerialNumber.String()
		o.Status.SerialNumber = &sn
		mod.Modify(true)
		mod.AssureStringValue(&o.Status.Message, "")
		mod.AssureBoolValue(&o.Status.Recreating, false)
		return mod.IsModified(), nil
	})

	return err
}

func (r *reconciler) resetForCertificatRecreation(certobj *api.RemoteAccessCertificate) error {
	_, _, err := r.certResources.ModifyStatus(certobj, func(data resources.ObjectData) (bool, error) {
		o := data.(*api.RemoteAccessCertificate)
		mod := utils.ModificationState{}
		mod.AssureStringValue(&o.Status.Message, "")
		mod.AssureBoolValue(&o.Status.Recreating, true)
		return mod.IsModified(), nil
	})
	if err != nil {
		return err
	}
	certobj.Spec.Recreate = false
	_, err = r.certResources.Update(certobj)

	return err
}

func (r *reconciler) Delete(logger logger.LogContext, obj resources.Object) reconcile.Status {
	cert := obj.Data().(*api.RemoteAccessCertificate)
	r.deleteSecret(cert.Namespace, cert.Spec.SecretName)
	return reconcile.Succeeded(logger)
}

func (r *reconciler) Deleted(logger logger.LogContext, key resources.ClusterObjectKey) reconcile.Status {
	return reconcile.Succeeded(logger)
}

func (r *reconciler) deleteSecret(namespace, name string) error {
	secret := &metav1.ObjectMeta{}
	secret.SetName(name)
	secret.SetNamespace(namespace)
	return r.secretResources.DeleteByName(secret)
}

func (r *reconciler) getSecret(namespace, name string) (*corev1.Secret, error) {
	secret := &corev1.Secret{}
	secret.SetName(name)
	secret.SetNamespace(namespace)
	obj, err := r.secretResources.Get(secret)
	if err != nil {
		return nil, err
	}
	return obj.Data().(*corev1.Secret), nil
}
