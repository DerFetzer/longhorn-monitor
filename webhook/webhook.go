package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	whhttp "github.com/slok/kubewebhook/pkg/http"
	"github.com/slok/kubewebhook/pkg/log"
	mutatingwh "github.com/slok/kubewebhook/pkg/webhook/mutating"
)

type config struct {
	certFile   string
	keyFile    string
	monitorSvc string
}

func initFlags() *config {
	cfg := &config{}

	fl := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	fl.StringVar(&cfg.certFile, "tls-cert-file", "", "TLS certificate file")
	fl.StringVar(&cfg.keyFile, "tls-key-file", "", "TLS key file")

	fl.Parse(os.Args[1:])

	if v, p := os.LookupEnv("MONITOR_SVC"); p {
		cfg.monitorSvc = v
	} else {
		panic("MONITOR_SVC environment variable has to be set!")
	}

	return cfg
}

func main() {
	logger := &log.Std{Debug: true}

	cfg := initFlags()

	// Create our mutator
	mt := mutatingwh.MutatorFunc(func(_ context.Context, obj metav1.Object) (bool, error) {
		pod, ok := obj.(*corev1.Pod)
		if !ok {
			// If not a pod just continue the mutation chain(if there is one) and don't do nothing.
			return false, nil
		}

		if pod.Annotations != nil {
			if name, ok := pod.Annotations["der-fetzer.de/longhorn-monitor.volume-name"]; ok {
				container := corev1.Container{
					Name:    "pvc-health-check",
					Image:   "derfetzer/longhorn-monitor:dev",
					Command: []string{"/usr/local/bin/longhorn-monitor/healthcheck"},
					Env: []corev1.EnvVar{
						corev1.EnvVar{
							Name: "NAMESPACE",
							ValueFrom: &corev1.EnvVarSource{
								FieldRef: &corev1.ObjectFieldSelector{
									FieldPath: "metadata.namespace",
								},
							},
						},
						corev1.EnvVar{
							Name:  "MONITOR_SVC",
							Value: cfg.monitorSvc,
						},
					},
					VolumeMounts: []corev1.VolumeMount{corev1.VolumeMount{MountPath: "/pvc", Name: name}},
				}

				pod.Spec.Containers = append(pod.Spec.Containers, container)
			}
		}

		return false, nil
	})

	mcfg := mutatingwh.WebhookConfig{
		Name: "podAnnotate",
		Obj:  &corev1.Pod{},
	}
	wh, err := mutatingwh.NewWebhook(mcfg, mt, nil, nil, logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating webhook: %s", err)
		os.Exit(1)
	}

	// Get the handler for our webhook.
	whHandler, err := whhttp.HandlerFor(wh)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating webhook handler: %s", err)
		os.Exit(1)
	}
	logger.Infof("Listening on :8080")
	err = http.ListenAndServeTLS(":8080", cfg.certFile, cfg.keyFile, whHandler)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error serving webhook: %s", err)
		os.Exit(1)
	}
}
