package main

import (
	"context"
	"io/ioutil"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/derfetzer/longhorn-monitor/healthcheck/v2/apiclient"
)

type HealthCheckConfig struct {
	Interval       uint32
	MonitorService string
}

type PodInfo struct {
	Name      string
	Namespace string
}

func initConfig() *HealthCheckConfig {
	cfg := &HealthCheckConfig{}

	if v, p := os.LookupEnv("MONITOR_SVC"); p {
		cfg.MonitorService = v
	} else {
		panic("MONITOR_SVC environment variable has to be set!")
	}

	if v, p := os.LookupEnv("INTERVAL"); p {
		if conv, err := strconv.ParseUint(v, 10, 32); err == nil {
			cfg.Interval = uint32(conv)
		} else {
			panic("INTERVAL environment variable could no be parsed ti unit32!")
		}
	} else {
		cfg.Interval = 60
	}

	return cfg
}

func initPodInfo() *PodInfo {
	podInfo := &PodInfo{}

	if v, p := os.LookupEnv("HOSTNAME"); p {
		podInfo.Name = v
	} else {
		panic("HOSTNAME environment variable has to be set!")
	}

	if v, p := os.LookupEnv("NAMESPACE"); p {
		podInfo.Namespace = v
	} else {
		panic("NAMSPACE environment variable has to be set!")
	}

	return podInfo
}

func checkPvc() bool {
	err := ioutil.WriteFile("/pvc/probe", []byte{0x42}, 0644)
	return err == nil
}

func main() {
	config := initConfig()
	podInfo := initPodInfo()

	client, err := apiclient.NewClient(config.MonitorService)
	if err != nil {
		panic("Could not create API client! " + err.Error())
	}

	sigCh := make(chan os.Signal)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	ticker := time.NewTicker(time.Duration(config.Interval) * time.Second)
	done := make(chan bool)

	go func() {
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				isHealthy := checkPvc()
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

				client.PostHealth(ctx, &apiclient.PostHealthParams{
					IsHealthy: isHealthy,
					PodName:   podInfo.Name,
					Namespace: podInfo.Namespace,
				})

				cancel()
			}
		}
	}()

	select {
	case <-sigCh:
		ticker.Stop()
		done <- true

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)

		client.DeleteHealth(ctx, &apiclient.DeleteHealthParams{
			PodName:   podInfo.Name,
			Namespace: podInfo.Namespace,
		})

		cancel()
	}
}
