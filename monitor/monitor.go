package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"

	"github.com/deepmap/oapi-codegen/pkg/middleware"
	"github.com/derfetzer/longhorn-monitor/monitor/v2/apiserver"

	"github.com/labstack/echo/v4"
	echomiddleware "github.com/labstack/echo/v4/middleware"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type MonitorConfig struct {
	RestartThreshold uint32
	Debug            bool
}

func initConfig() *MonitorConfig {
	cfg := &MonitorConfig{}

	restartThreshold := os.Getenv("RESTART_THRESHOLD")
	if v, err := strconv.ParseUint(restartThreshold, 10, 32); err == nil {
		cfg.RestartThreshold = uint32(v)
	} else {
		panic("RESTART_THRESHOLD environment variable has to be set!")
	}

	debug := os.Getenv("DEBUG")
	if v, err := strconv.ParseBool(debug); err == nil {
		cfg.Debug = v
	} else {
		cfg.Debug = false
	}

	return cfg
}

func initKubernetes() kubernetes.Interface {
	// creates the in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}
	// creates the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	return clientset
}

func initWebServer(healthMonitor *apiserver.HealthMonitor) *echo.Echo {
	swagger, err := apiserver.GetSwagger()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading swagger spec\n: %s", err)
		os.Exit(1)
	}

	// Clear out the servers array in the swagger spec, that skips validating
	// that server names match. We don't know how this thing will be run.
	swagger.Servers = nil

	// This is how you set up a basic Echo router
	e := echo.New()

	// Log all requests
	e.Use(echomiddleware.Logger())
	// Use our validation middleware to check all requests against the
	// OpenAPI schema.
	e.Use(middleware.OapiRequestValidator(swagger))

	// We now register our healthMonitor above as the handler for the interface
	apiserver.RegisterHandlers(e, healthMonitor)

	return e
}

func initHealthMonitor(podDeletes chan<- apiserver.PodIdentifier, config *MonitorConfig) *apiserver.HealthMonitor {
	return apiserver.NewHealthMonitor(podDeletes, config.RestartThreshold)
}

func deletePod(podDeletes <-chan apiserver.PodIdentifier, clientset kubernetes.Interface) {
	for {
		podIdentifier := <-podDeletes
		podClient := clientset.CoreV1().Pods(podIdentifier.Namespace)
		err := podClient.Delete(podIdentifier.Name, &metav1.DeleteOptions{})
		if errors.IsNotFound(err) {
			fmt.Printf("Pod %v not found\n", podIdentifier.Name)
		} else if statusError, isStatus := err.(*errors.StatusError); isStatus {
			fmt.Printf("Error deleting pod %v\n", statusError.ErrStatus.Message)
		} else if err != nil {
			panic(err.Error())
		} else {
			fmt.Printf("Pod %v deleted\n", podIdentifier.Name)
		}
	}
}

func main() {
	var port = flag.Int("port", 8080, "Port for HTTP server")
	flag.Parse()

	config := initConfig()
	clientset := initKubernetes()
	podDeletes := make(chan apiserver.PodIdentifier)
	healthMonitor := initHealthMonitor(podDeletes, config)
	e := initWebServer(healthMonitor)

	if config.Debug {
		e.Debug = true
	}

	go deletePod(podDeletes, clientset)

	// And we serve HTTP until the world ends.
	e.Logger.Fatal(e.Start(fmt.Sprintf("0.0.0.0:%d", *port)))
}
