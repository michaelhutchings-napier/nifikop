package accesspolicies

import (
	"testing"

	nigoapi "github.com/konpyutaika/nigoapi/pkg/nifi"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/konpyutaika/nifikop/api/v1"
)

func TestWithManagedNodesForDataPolicy(t *testing.T) {
	restricted := nifiUserGroup("restricted", "restricted-id")
	managedNodes := nifiUserGroup("cluster.managed-nodes", "managed-nodes-id")

	tests := []struct {
		name       string
		policy     v1.AccessPolicy
		wantGroups []string
	}{
		{
			name: "component data read includes managed nodes",
			policy: v1.AccessPolicy{
				Type:          v1.ComponentAccessPolicyType,
				Action:        v1.ReadAccessPolicyAction,
				Resource:      v1.DataAccessPolicyResource,
				ComponentType: v1.ProcessGroupType,
				ComponentId:   "pg-1",
			},
			wantGroups: []string{"restricted-id", "managed-nodes-id"},
		},
		{
			name: "component data write includes managed nodes",
			policy: v1.AccessPolicy{
				Type:          v1.ComponentAccessPolicyType,
				Action:        v1.WriteAccessPolicyAction,
				Resource:      v1.DataAccessPolicyResource,
				ComponentType: v1.ProcessGroupType,
				ComponentId:   "pg-1",
			},
			wantGroups: []string{"restricted-id", "managed-nodes-id"},
		},
		{
			name: "component non-data does not include managed nodes",
			policy: v1.AccessPolicy{
				Type:          v1.ComponentAccessPolicyType,
				Action:        v1.ReadAccessPolicyAction,
				Resource:      v1.OperationAccessPolicyResource,
				ComponentType: v1.ProcessGroupType,
				ComponentId:   "pg-1",
			},
			wantGroups: []string{"restricted-id"},
		},
		{
			name: "global data-like policy does not include managed nodes",
			policy: v1.AccessPolicy{
				Type:     v1.GlobalAccessPolicyType,
				Action:   v1.ReadAccessPolicyAction,
				Resource: v1.DataAccessPolicyResource,
			},
			wantGroups: []string{"restricted-id"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			groups := WithManagedNodesForDataPolicy(&tt.policy, []*v1.NifiUserGroup{restricted}, managedNodes)
			assertUserGroupIds(t, groups, tt.wantGroups)
		})
	}
}

func TestWithManagedNodesForDataPolicyDoesNotDuplicateManagedNodes(t *testing.T) {
	restricted := nifiUserGroup("restricted", "restricted-id")
	managedNodes := nifiUserGroup("cluster.managed-nodes", "managed-nodes-id")
	policy := v1.AccessPolicy{
		Type:          v1.ComponentAccessPolicyType,
		Action:        v1.ReadAccessPolicyAction,
		Resource:      v1.DataAccessPolicyResource,
		ComponentType: v1.ProcessGroupType,
		ComponentId:   "pg-1",
	}

	groups := WithManagedNodesForDataPolicy(&policy, []*v1.NifiUserGroup{restricted, managedNodes}, managedNodes)
	groups = WithManagedNodesForDataPolicy(&policy, groups, managedNodes)

	assertUserGroupIds(t, groups, []string{"restricted-id", "managed-nodes-id"})
}

func TestAddRemoveUserGroupsFromAccessPolicyEntityIsIdempotent(t *testing.T) {
	restricted := nifiUserGroup("restricted", "restricted-id")
	managedNodes := nifiUserGroup("cluster.managed-nodes", "managed-nodes-id")
	entity := &nigoapi.AccessPolicyEntity{
		Component: &nigoapi.AccessPolicyDto{},
	}

	addRemoveUserGroupsFromAccessPolicyEntity(
		[]*v1.NifiUserGroup{restricted, managedNodes, managedNodes},
		[]*v1.NifiUserGroup{},
		entity,
	)
	addRemoveUserGroupsFromAccessPolicyEntity(
		[]*v1.NifiUserGroup{restricted, managedNodes},
		[]*v1.NifiUserGroup{},
		entity,
	)

	if len(entity.Component.UserGroups) != 2 {
		t.Fatalf("expected 2 unique user groups, got %d: %#v", len(entity.Component.UserGroups), entity.Component.UserGroups)
	}
}

func TestManagedNodesShouldKeepDataPolicy(t *testing.T) {
	managedNodes := nifiUserGroup("cluster.managed-nodes", "managed-nodes-id")
	restricted := nifiUserGroup("restricted", "restricted-id")

	if !ManagedNodesShouldKeepDataPolicy(managedNodes, "read", "/data/process-groups/pg-1") {
		t.Fatal("expected managed-nodes to keep component data read policy")
	}
	if !ManagedNodesShouldKeepDataPolicy(managedNodes, "write", "/data/process-groups/pg-1") {
		t.Fatal("expected managed-nodes to keep component data write policy")
	}
	if ManagedNodesShouldKeepDataPolicy(managedNodes, "read", "/operation/process-groups/pg-1") {
		t.Fatal("did not expect managed-nodes to keep non-data policy")
	}
	if ManagedNodesShouldKeepDataPolicy(restricted, "read", "/data/process-groups/pg-1") {
		t.Fatal("did not expect non-managed-nodes group to keep data policy")
	}
}

func nifiUserGroup(name, id string) *v1.NifiUserGroup {
	return &v1.NifiUserGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "nifi",
		},
		Status: v1.NifiUserGroupStatus{
			Id: id,
		},
	}
}

func assertUserGroupIds(t *testing.T, groups []*v1.NifiUserGroup, want []string) {
	t.Helper()
	if len(groups) != len(want) {
		t.Fatalf("expected %d groups, got %d", len(want), len(groups))
	}
	for i, group := range groups {
		if group.Status.Id != want[i] {
			t.Fatalf("group %d: expected %q, got %q", i, want[i], group.Status.Id)
		}
	}
}
