package nifi

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	v1 "github.com/konpyutaika/nifikop/api/v1"
	"github.com/konpyutaika/nifikop/pkg/util"
)

func TestInjectAdditionalSecurityContextMergesContainerOverrides(t *testing.T) {
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
	}
	injected := rec.injectAdditionalSecurityContext(&v1.NodeConfig{
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: util.BoolPointer(false),
			RunAsNonRoot:             util.BoolPointer(true),
			RunAsGroup:               &runAsGroup,
		},
	}, containers)

	require.Len(t, injected, 2)

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
	assert.Equal(t, runAsGroup, *byName["storage-manager"].SecurityContext.RunAsGroup)
	assert.Nil(t, byName["storage-manager"].SecurityContext.RunAsNonRoot)
	assert.Nil(t, byName["storage-manager"].SecurityContext.AllowPrivilegeEscalation)
}
