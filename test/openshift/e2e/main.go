package main

import (
	"context"
	"flag"

	"github.com/golang/glog"
	mapiv1alpha1 "github.com/openshift/cluster-api/pkg/apis/machine/v1beta1"
	caov1alpha1 "github.com/openshift/cluster-autoscaler-operator/pkg/apis"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"
)

const (
	namespace = "kube-system"
)

func init() {
	if err := mapiv1alpha1.AddToScheme(scheme.Scheme); err != nil {
		glog.Fatal(err)
	}

	if err := caov1alpha1.AddToScheme(scheme.Scheme); err != nil {
		glog.Fatal(err)
	}
}

type testConfig struct {
	client client.Client
}

func newClient() (client.Client, error) {
	// Get a config to talk to the apiserver
	cfg, err := config.GetConfig()
	if err != nil {
		return nil, err
	}

	return client.New(cfg, client.Options{})

}

func main() {
	flag.Parse()
	if err := runSuite(signals.SetupSignalHandler()); err != nil {
		glog.Fatal(err)
	}
}

func runSuite(stopCh <-chan struct{}) error {
	client, err := newClient()
	if err != nil {
		return err
	}
	testConfig := &testConfig{
		client: client,
	}

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		select {
		case <-stopCh:
			cancel()
		case <-ctx.Done():
		}
	}()

	if false {
		glog.Info("RUN: ExpectOperatorAvailable")
		if err := testConfig.ExpectOperatorAvailable(ctx); err != nil {
			glog.Errorf("FAIL: ExpectOperatorAvailable: %v", err)
			return err
		}
		glog.Info("PASS: ExpectOperatorAvailable")
	}

	glog.Info("RUN: ExpectAutoscalerScalesOut")
	if err := testConfig.ExpectAutoscalerScalesOut(ctx); err != nil {
		glog.Errorf("FAIL: ExpectAutoscalerScalesOut: %v", err)
		return err
	}
	glog.Info("PASS: ExpectAutoscalerScalesOut")
	return nil
}
