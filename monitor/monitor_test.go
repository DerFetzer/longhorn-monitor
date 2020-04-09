package main

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/derfetzer/longhorn-monitor/monitor/v2/apiserver"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/deepmap/oapi-codegen/pkg/testutil"
	"github.com/stretchr/testify/assert"
)

func TestHealthMonitor(t *testing.T) {
	assert := assert.New(t)

	podDeletes := make(chan apiserver.PodIdentifier, 1)
	deleteResults := make(chan apiserver.PodDeleteResult, 1)

	healthMonitor := initHealthMonitor(podDeletes, deleteResults, &MonitorConfig{RestartThreshold: 4})

	e := initWebServer(healthMonitor)

	q := make(url.Values)
	q.Set("podName", "testPod")
	q.Set("namespace", "default")
	q.Set("isHealthy", "true")

	result := testutil.NewRequest().Post("/podHealth?"+q.Encode()).Go(t, e)
	assert.Equal(http.StatusCreated, result.Code())
	assert.Empty(podDeletes)

	q = make(url.Values)
	q.Set("podName", "testPod2")
	q.Set("namespace", "default")
	q.Set("isHealthy", "true")

	result = testutil.NewRequest().Post("/podHealth?"+q.Encode()).Go(t, e)
	assert.Equal(http.StatusCreated, result.Code())

	result = testutil.NewRequest().Get("/podHealth").Go(t, e)
	var resultList []apiserver.PodHealth
	assert.Equal(http.StatusOK, result.Code())
	err := result.UnmarshalBodyToObject(&resultList)
	assert.NoError(err, "error unmarshaling response")
	assert.Equal(2, len(resultList))
	assert.Contains(resultList, apiserver.PodHealth{
		ErrorCount: 0,
		IsHealthy:  true,
		PodName:    "testPod",
		Namespace:  "default",
		IsDeleted:  false,
	})
	assert.Contains(resultList, apiserver.PodHealth{
		ErrorCount: 0,
		IsHealthy:  true,
		PodName:    "testPod2",
		Namespace:  "default",
		IsDeleted:  false,
	})
	assert.Empty(podDeletes)

	q = make(url.Values)
	q.Set("podName", "testPod")
	q.Set("namespace", "default")
	q.Set("isHealthy", "false")

	for i := 0; i < 3; i++ {
		result := testutil.NewRequest().Post("/podHealth?"+q.Encode()).Go(t, e)
		assert.Equal(http.StatusOK, result.Code())
	}

	result = testutil.NewRequest().Get("/podHealth").Go(t, e)
	var resultList2 []apiserver.PodHealth
	assert.Equal(http.StatusOK, result.Code())
	err = result.UnmarshalBodyToObject(&resultList2)
	assert.NoError(err, "error unmarshaling response")
	assert.Equal(2, len(resultList2))
	assert.Contains(resultList2, apiserver.PodHealth{
		ErrorCount: 3,
		IsHealthy:  false,
		PodName:    "testPod",
		Namespace:  "default",
		IsDeleted:  false,
	})
	assert.Contains(resultList2, apiserver.PodHealth{
		ErrorCount: 0,
		IsHealthy:  true,
		PodName:    "testPod2",
		Namespace:  "default",
		IsDeleted:  false,
	})
	assert.Empty(podDeletes)

	result = testutil.NewRequest().Post("/podHealth?"+q.Encode()).Go(t, e)
	assert.Equal(http.StatusOK, result.Code())

	if assert.NotEmpty(podDeletes) {
		assert.Equal(1, len(podDeletes))
		assert.Equal(apiserver.PodIdentifier{Name: "testPod", Namespace: "default"}, <-podDeletes)
	}

	// Simulate successful delete
	deleteResults <- apiserver.PodDeleteResult{
		Identifier: apiserver.PodIdentifier{
			Name:      "testPod",
			Namespace: "default",
		},
		Success: true,
	}

	time.Sleep(100 * time.Millisecond)

	result = testutil.NewRequest().Get("/podHealth").Go(t, e)
	var resultList3 []apiserver.PodHealth
	assert.Equal(http.StatusOK, result.Code())
	err = result.UnmarshalBodyToObject(&resultList3)
	assert.NoError(err, "error unmarshaling response")
	assert.Equal(2, len(resultList3))
	assert.Contains(resultList3, apiserver.PodHealth{
		ErrorCount: 4,
		IsHealthy:  false,
		PodName:    "testPod",
		Namespace:  "default",
		IsDeleted:  true,
	})
	assert.Contains(resultList3, apiserver.PodHealth{
		ErrorCount: 0,
		IsHealthy:  true,
		PodName:    "testPod2",
		Namespace:  "default",
		IsDeleted:  false,
	})
	assert.Empty(podDeletes)

	q = make(url.Values)
	q.Set("podName", "testPod")
	q.Set("namespace", "default")
	q.Set("isHealthy", "true")

	result = testutil.NewRequest().Post("/podHealth?"+q.Encode()).Go(t, e)
	assert.Equal(http.StatusInternalServerError, result.Code())

	result = testutil.NewRequest().Get("/podHealth").Go(t, e)
	var resultList4 []apiserver.PodHealth
	assert.Equal(http.StatusOK, result.Code())
	err = result.UnmarshalBodyToObject(&resultList4)
	assert.NoError(err, "error unmarshaling response")
	assert.Equal(2, len(resultList4))
	assert.Contains(resultList4, apiserver.PodHealth{
		ErrorCount: 4,
		IsHealthy:  false,
		PodName:    "testPod",
		Namespace:  "default",
		IsDeleted:  true,
	})
	assert.Contains(resultList4, apiserver.PodHealth{
		ErrorCount: 0,
		IsHealthy:  true,
		PodName:    "testPod2",
		Namespace:  "default",
		IsDeleted:  false,
	})
	assert.Empty(podDeletes)

	q = make(url.Values)
	q.Set("podName", "testPod")
	q.Set("namespace", "default")

	result = testutil.NewRequest().Delete("/podHealth?"+q.Encode()).Go(t, e)
	assert.Equal(http.StatusOK, result.Code())

	result = testutil.NewRequest().Get("/podHealth").Go(t, e)
	var resultList5 []apiserver.PodHealth
	assert.Equal(http.StatusOK, result.Code())
	err = result.UnmarshalBodyToObject(&resultList5)
	assert.NoError(err, "error unmarshaling response")
	assert.Equal(1, len(resultList5))
	assert.Contains(resultList5, apiserver.PodHealth{
		ErrorCount: 0,
		IsHealthy:  true,
		PodName:    "testPod2",
		Namespace:  "default",
		IsDeleted:  false,
	})
	assert.Empty(podDeletes)
}

func TestDeletePod(t *testing.T) {
	assert := assert.New(t)

	clientset := fake.NewSimpleClientset(
		&v1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "testPod", Namespace: "default"},
		},
		&v1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "testPod2", Namespace: "default"},
		},
	)

	podDeletes := make(chan apiserver.PodIdentifier)
	deleteResults := make(chan apiserver.PodDeleteResult, 1)
	go deletePod(podDeletes, deleteResults, clientset)

	l, _ := clientset.CoreV1().Pods("").List(metav1.ListOptions{})
	assert.Equal(2, len(l.Items))

	podDeletes <- apiserver.PodIdentifier{
		Name:      "unknown",
		Namespace: "default",
	}

	time.Sleep(100 * time.Millisecond)

	if assert.NotEmpty(deleteResults) {
		assert.Equal(1, len(deleteResults))
		assert.Equal(apiserver.PodDeleteResult{
			Identifier: apiserver.PodIdentifier{
				Name:      "unknown",
				Namespace: "default",
			},
			Success: false}, <-deleteResults)
	}

	l, _ = clientset.CoreV1().Pods("").List(metav1.ListOptions{})
	assert.Equal(2, len(l.Items))

	podDeletes <- apiserver.PodIdentifier{
		Name:      "testPod",
		Namespace: "default",
	}

	time.Sleep(100 * time.Millisecond)

	if assert.NotEmpty(deleteResults) {
		assert.Equal(1, len(deleteResults))
		assert.Equal(apiserver.PodDeleteResult{
			Identifier: apiserver.PodIdentifier{
				Name:      "testPod",
				Namespace: "default",
			},
			Success: true}, <-deleteResults)
	}

	l, _ = clientset.CoreV1().Pods("").List(metav1.ListOptions{})
	assert.Equal(1, len(l.Items))
	assert.Equal("testPod2", l.Items[0].GetName())
}
