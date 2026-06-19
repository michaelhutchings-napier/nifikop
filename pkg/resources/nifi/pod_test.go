package nifi

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/konpyutaika/nifikop/api/v1"
	"github.com/konpyutaika/nifikop/pkg/errorfactory"
	"github.com/konpyutaika/nifikop/pkg/resources"
	"github.com/konpyutaika/nifikop/pkg/util"
	nifiutil "github.com/konpyutaika/nifikop/pkg/util/nifi"
	"go.uber.org/zap"
)

func TestInjectAdditionalSecurityContextPreservesContainerOverrides(t *testing.T) {
	runAsRoot := int64(0)
	runAsGroup := int64(2)

	rec := Reconciler{}
	containers := []corev1.Container{
		{Name: "nifi"},
		{
			Name: "storage-manager",
			SecurityContext: &corev1.SecurityContext{
				Privileged: util.BoolPointer(true),
				RunAsUser:  &runAsRoot,
			},
		},
		{
			Name: "object-sync",
			SecurityContext: &corev1.SecurityContext{
				AllowPrivilegeEscalation: util.BoolPointer(false),
				RunAsNonRoot:             util.BoolPointer(true),
			},
		},
	}
	injected := rec.injectAdditionalSecurityContext(&v1.NodeConfig{
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: util.BoolPointer(false),
			RunAsNonRoot:             util.BoolPointer(true),
			RunAsGroup:               &runAsGroup,
		},
	}, containers)

	require.Len(t, injected, 3)

	byName := map[string]corev1.Container{}
	for _, container := range injected {
		byName[container.Name] = container
	}

	require.NotNil(t, byName["nifi"].SecurityContext)
	assert.Nil(t, byName["nifi"].SecurityContext.RunAsUser)
	assert.Equal(t, runAsGroup, *byName["nifi"].SecurityContext.RunAsGroup)
	assert.False(t, *byName["nifi"].SecurityContext.AllowPrivilegeEscalation)

	require.NotNil(t, byName["storage-manager"].SecurityContext)
	assert.True(t, *byName["storage-manager"].SecurityContext.Privileged)
	assert.Equal(t, runAsRoot, *byName["storage-manager"].SecurityContext.RunAsUser)
	assert.Nil(t, byName["storage-manager"].SecurityContext.RunAsNonRoot)
	assert.Nil(t, byName["storage-manager"].SecurityContext.AllowPrivilegeEscalation)

	require.NotNil(t, byName["object-sync"].SecurityContext)
	assert.Nil(t, byName["object-sync"].SecurityContext.Privileged)
	assert.Nil(t, byName["object-sync"].SecurityContext.RunAsUser)
	assert.Nil(t, byName["object-sync"].SecurityContext.RunAsGroup)
	assert.True(t, *byName["object-sync"].SecurityContext.RunAsNonRoot)
	assert.False(t, *byName["object-sync"].SecurityContext.AllowPrivilegeEscalation)
}

func TestPodAppliesNodeConfigSELinuxOptions(t *testing.T) {
	rec := Reconciler{
		Reconciler: resources.Reconciler{
			NifiCluster: &v1.NifiCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cluster",
					Namespace: "namespace",
				},
				Spec: v1.NifiClusterSpec{
					ListenersConfig: &v1.ListenersConfig{
						InternalListeners: []v1.InternalListenerConfig{
							{
								Type:          v1.HttpListenerType,
								Name:          "http",
								ContainerPort: 8080,
							},
						},
					},
				},
			},
		},
	}

	pod := rec.pod(v1.Node{Id: 1}, &v1.NodeConfig{
		SELinuxOptions: &corev1.SELinuxOptions{
			Level: "s0:c572,c681",
		},
	}, nil, *zap.NewNop()).(*corev1.Pod)

	require.NotNil(t, pod.Spec.SecurityContext)
	require.NotNil(t, pod.Spec.SecurityContext.SELinuxOptions)
	assert.Equal(t, "s0:c572,c681", pod.Spec.SecurityContext.SELinuxOptions.Level)
}

func TestReconcileNifiPodKeepsNotReadyPodDuringGracefulAction(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	currentPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nifi-cluster-1-nodeabcde",
			Namespace: "namespace",
			Labels: map[string]string{
				"app":     "nifi",
				"nifi_cr": "cluster",
				"nodeId":  "1",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "nifi", Image: "apache/nifi:old"},
			},
			NodeName: "node-1",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodFailed,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionFalse},
			},
		},
	}
	desiredPod := currentPod.DeepCopy()
	desiredPod.Spec.Containers[0].Image = "apache/nifi:new"

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(currentPod).Build()
	cluster := &v1.NifiCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster",
			Namespace: "namespace",
		},
		Status: v1.NifiClusterStatus{
			NodesState: map[string]v1.NodeState{
				"1": {
					ConfigurationState: v1.ConfigInSync,
					GracefulActionState: v1.GracefulActionState{
						State: v1.GracefulUpscaleRunning,
					},
				},
			},
		},
	}
	rec := Reconciler{
		Reconciler: resources.Reconciler{
			Client:      client,
			NifiCluster: cluster,
		},
	}

	err, ready := rec.reconcileNifiPod(*zap.NewNop(), desiredPod)

	require.Error(t, err)
	_, isRollingUpgrade := err.(errorfactory.ReconcileRollingUpgrade)
	assert.True(t, isRollingUpgrade)
	assert.False(t, ready)

	preservedPod := &corev1.Pod{}
	require.NoError(t, client.Get(context.Background(), types.NamespacedName{
		Name:      currentPod.Name,
		Namespace: currentPod.Namespace,
	}, preservedPod))
	assert.Equal(t, "apache/nifi:old", preservedPod.Spec.Containers[0].Image)
}

func TestReconcileNifiPodMarksStalledGracefulAction(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, v1.AddToScheme(scheme))

	currentPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nifi-cluster-1-nodeabcde",
			Namespace: "namespace",
			Labels: map[string]string{
				"app":     "nifi",
				"nifi_cr": "cluster",
				"nodeId":  "1",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "nifi", Image: "apache/nifi:old"},
			},
			NodeName: "node-1",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodFailed,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionFalse},
			},
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "storage-manager",
					State: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{Reason: "Error"},
					},
				},
			},
		},
	}
	desiredPod := currentPod.DeepCopy()
	desiredPod.Spec.Containers[0].Image = "apache/nifi:new"

	cluster := &v1.NifiCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster",
			Namespace: "namespace",
		},
		Spec: v1.NifiClusterSpec{
			NifiClusterTaskSpec: v1.NifiClusterTaskSpec{RetryDurationMinutes: 5},
		},
		Status: v1.NifiClusterStatus{
			NodesState: map[string]v1.NodeState{
				"1": {
					ConfigurationState: v1.ConfigInSync,
					GracefulActionState: v1.GracefulActionState{
						ActionStep:  v1.ConnectStatus,
						State:       v1.GracefulUpscaleRunning,
						TaskStarted: time.Now().Add(-10 * time.Minute).UTC().Format(nifiutil.TimeStampLayout),
					},
				},
			},
		},
	}
	currentStatus := cluster.DeepCopy().Status
	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(currentPod, cluster).
		WithStatusSubresource(cluster).
		Build()
	recorder := record.NewFakeRecorder(1)
	rec := Reconciler{
		Recorder: recorder,
		Reconciler: resources.Reconciler{
			Client:                   client,
			NifiCluster:              cluster,
			NifiClusterCurrentStatus: currentStatus,
		},
	}

	err, ready := rec.reconcileNifiPod(*zap.NewNop(), desiredPod)

	require.Error(t, err)
	_, isRollingUpgrade := err.(errorfactory.ReconcileRollingUpgrade)
	assert.True(t, isRollingUpgrade)
	assert.False(t, ready)

	preservedPod := &corev1.Pod{}
	require.NoError(t, client.Get(context.Background(), types.NamespacedName{
		Name:      currentPod.Name,
		Namespace: currentPod.Namespace,
	}, preservedPod))
	assert.Equal(t, "apache/nifi:old", preservedPod.Spec.Containers[0].Image)

	updatedCluster := &v1.NifiCluster{}
	require.NoError(t, client.Get(context.Background(), types.NamespacedName{
		Name:      cluster.Name,
		Namespace: cluster.Namespace,
	}, updatedCluster))
	message := updatedCluster.Status.NodesState["1"].GracefulActionState.ErrorMessage
	assert.Contains(t, message, "is not ready after 5m graceful action timeout")
	assert.Contains(t, message, "storage-manager terminated=Error")

	select {
	case event := <-recorder.Events:
		assert.True(t, strings.Contains(event, gracefulActionPodStalledReason), event)
	default:
		t.Fatal("expected stalled graceful action event")
	}
}
