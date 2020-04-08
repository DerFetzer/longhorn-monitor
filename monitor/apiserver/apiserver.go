package apiserver

import (
	"net/http"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
)

type HealthStatus struct {
	ErrorCount uint32
	LastSeen   time.Time
	IsDeleted  bool
}

type PodIdentifier struct {
	Name      string
	Namespace string
}

type HealthMonitor struct {
	Pods           map[PodIdentifier]*HealthStatus
	PodDeletes     chan<- PodIdentifier
	ErrorThreshold uint32
	Lock           sync.Mutex
}

func NewHealthMonitor(podDeletes chan<- PodIdentifier, errorThreshold uint32) *HealthMonitor {
	return &HealthMonitor{
		Pods:           make(map[PodIdentifier]*HealthStatus),
		PodDeletes:     podDeletes,
		ErrorThreshold: errorThreshold,
	}
}

func (hm *HealthMonitor) PostHealth(ctx echo.Context, params PostHealthParams) error {
	hm.Lock.Lock()
	defer hm.Lock.Unlock()

	podIdentifier := PodIdentifier{
		Name:      params.PodName,
		Namespace: params.Namespace,
	}

	if healthStatus, p := hm.Pods[podIdentifier]; p {
		if params.IsHealthy {
			healthStatus.ErrorCount = 0
		} else {
			healthStatus.ErrorCount++
		}
		healthStatus.LastSeen = time.Now()
		if healthStatus.ErrorCount >= hm.ErrorThreshold && !healthStatus.IsDeleted {
			healthStatus.IsDeleted = true
			hm.PodDeletes <- podIdentifier
		}
		return ctx.NoContent(http.StatusOK)
	}

	if params.IsHealthy {
		hm.Pods[podIdentifier] = &HealthStatus{ErrorCount: 0, LastSeen: time.Now()}
	} else {
		hm.Pods[podIdentifier] = &HealthStatus{ErrorCount: 1, LastSeen: time.Now()}
	}
	return ctx.NoContent(http.StatusCreated)
}

func (hm *HealthMonitor) GetHealth(ctx echo.Context) error {
	hm.Lock.Lock()
	defer hm.Lock.Unlock()

	var result []PodHealth

	for podIdentifier, healthStatus := range hm.Pods {
		result = append(result, PodHealth{
			PodName:    podIdentifier.Name,
			Namespace:  podIdentifier.Namespace,
			IsHealthy:  healthStatus.ErrorCount == 0,
			ErrorCount: int32(healthStatus.ErrorCount),
			IsDeleted:  healthStatus.IsDeleted})
	}

	return ctx.JSON(http.StatusOK, result)
}

func (hm *HealthMonitor) DeleteHealth(ctx echo.Context, params DeleteHealthParams) error {
	hm.Lock.Lock()
	defer hm.Lock.Unlock()

	podIdentifier := PodIdentifier{
		Name:      params.PodName,
		Namespace: params.Namespace,
	}

	if _, p := hm.Pods[podIdentifier]; p {
		delete(hm.Pods, podIdentifier)
		return ctx.NoContent(http.StatusOK)
	}

	return ctx.NoContent(http.StatusNotFound)
}
