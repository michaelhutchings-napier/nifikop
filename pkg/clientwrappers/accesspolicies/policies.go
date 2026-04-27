package accesspolicies

import (
	"strings"

	nigoapi "github.com/konpyutaika/nigoapi/pkg/nifi"
	"k8s.io/apimachinery/pkg/types"

	v1 "github.com/konpyutaika/nifikop/api/v1"
	"github.com/konpyutaika/nifikop/pkg/clientwrappers"
	"github.com/konpyutaika/nifikop/pkg/common"
	"github.com/konpyutaika/nifikop/pkg/nificlient"
	"github.com/konpyutaika/nifikop/pkg/util/clientconfig"
)

var log = common.CustomLogger().Named("accesspolicies-method")

type ManagedUserGroups struct {
	Nodes   *v1.NifiUserGroup
	Admins  *v1.NifiUserGroup
	Readers *v1.NifiUserGroup
}

func ExistAccessPolicies(accessPolicy *v1.AccessPolicy, config *clientconfig.NifiConfig) (bool, error) {
	nClient, err := common.NewClusterConnection(log, config)
	if err != nil {
		return false, err
	}

	entity, err := nClient.GetAccessPolicy(string(accessPolicy.Action), accessPolicy.GetResource(config.RootProcessGroupId))
	if err := clientwrappers.ErrorGetOperation(log, err, "Get access policy"); err != nil {
		if err == nificlient.ErrNifiClusterReturned404 {
			return false, nil
		}
		return false, err
	}

	return entity != nil && entity.Component.Resource == string(accessPolicy.GetResource(config.RootProcessGroupId)), nil
}

func CreateAccessPolicy(accessPolicy *v1.AccessPolicy, config *clientconfig.NifiConfig) (string, error) {
	nClient, err := common.NewClusterConnection(log, config)
	if err != nil {
		return "", err
	}

	scratchEntity := nigoapi.AccessPolicyEntity{}
	updateAccessPolicyEntity(
		accessPolicy,
		[]*v1.NifiUser{}, []*v1.NifiUser{},
		[]*v1.NifiUserGroup{}, []*v1.NifiUserGroup{},
		config,
		&scratchEntity)

	entity, err := nClient.CreateAccessPolicy(scratchEntity)
	if err := clientwrappers.ErrorCreateOperation(log, err, "Access policy user"); err != nil {
		return "", err
	}

	return entity.Id, nil
}

func UpdateAccessPolicy(
	accessPolicy *v1.AccessPolicy,
	addUsers []*v1.NifiUser,
	removeUsers []*v1.NifiUser,
	addUserGroups []*v1.NifiUserGroup,
	removeUserGroups []*v1.NifiUserGroup,
	config *clientconfig.NifiConfig) error {
	nClient, err := common.NewClusterConnection(log, config)
	if err != nil {
		return err
	}

	// Check if the access policy exist
	exist, err := ExistAccessPolicies(accessPolicy, config)
	if err != nil {
		return err
	}

	if !exist {
		_, err := CreateAccessPolicy(accessPolicy, config)
		if err != nil {
			return err
		}
	}

	entity, err := nClient.GetAccessPolicy(string(accessPolicy.Action), accessPolicy.GetResource(config.RootProcessGroupId))
	if err := clientwrappers.ErrorGetOperation(log, err, "Get access policy"); err != nil {
		return err
	}

	updateAccessPolicyEntity(accessPolicy, addUsers, removeUsers, addUserGroups, removeUserGroups, config, entity)
	_, _ = nClient.UpdateAccessPolicy(*entity)
	return clientwrappers.ErrorUpdateOperation(log, err, "Update user")
}

func UpdateAccessPolicyEntity(
	entity *nigoapi.AccessPolicyEntity,
	addUsers []*v1.NifiUser,
	removeUsers []*v1.NifiUser,
	addUserGroups []*v1.NifiUserGroup,
	removeUserGroups []*v1.NifiUserGroup,
	config *clientconfig.NifiConfig) error {
	nClient, err := common.NewClusterConnection(log, config)
	if err != nil {
		return err
	}

	entity, err = nClient.GetAccessPolicy(entity.Component.Action, entity.Component.Resource)
	if err := clientwrappers.ErrorGetOperation(log, err, "Get access policy"); err != nil {
		return err
	}

	addRemoveUsersFromAccessPolicyEntity(addUsers, removeUsers, entity)
	addRemoveUserGroupsFromAccessPolicyEntity(addUserGroups, removeUserGroups, entity)

	_, _ = nClient.UpdateAccessPolicy(*entity)
	return clientwrappers.ErrorUpdateOperation(log, err, "Update user")
}

func updateAccessPolicyEntity(
	accessPolicy *v1.AccessPolicy,
	addUsers []*v1.NifiUser,
	removeUsers []*v1.NifiUser,
	addUserGroups []*v1.NifiUserGroup,
	removeUserGroups []*v1.NifiUserGroup,
	config *clientconfig.NifiConfig,
	entity *nigoapi.AccessPolicyEntity) {
	var defaultVersion int64 = 0

	if entity == nil {
		entity = &nigoapi.AccessPolicyEntity{}
	}

	if entity.Component == nil {
		entity.Revision = &nigoapi.RevisionDto{
			Version: &defaultVersion,
		}
	}

	if entity.Component == nil {
		entity.Component = &nigoapi.AccessPolicyDto{}
	}

	entity.Component.Action = string(accessPolicy.Action)
	entity.Component.Resource = accessPolicy.GetResource(config.RootProcessGroupId)

	addRemoveUsersFromAccessPolicyEntity(addUsers, removeUsers, entity)
	addRemoveUserGroupsFromAccessPolicyEntity(addUserGroups, removeUserGroups, entity)
}

func addRemoveUserGroupsFromAccessPolicyEntity(
	addUserGroups []*v1.NifiUserGroup,
	removeUserGroups []*v1.NifiUserGroup,
	entity *nigoapi.AccessPolicyEntity) {
	// Add new userGroup from the access policy
	for _, userGroup := range addUserGroups {
		addUserGroupToAccessPolicyEntity(userGroup, entity)
	}

	// Remove user from the access policy
	var userGroupsAccessPolicy []nigoapi.TenantEntity
	for _, userGroup := range entity.Component.UserGroups {
		contains := false

		for _, toRemove := range removeUserGroups {
			if userGroup.Id == toRemove.Status.Id {
				contains = true
				break
			}
		}

		if !contains {
			userGroupsAccessPolicy = append(userGroupsAccessPolicy, userGroup)
		}
	}
	entity.Component.UserGroups = userGroupsAccessPolicy
}

func addRemoveUsersFromAccessPolicyEntity(
	addUsers []*v1.NifiUser,
	removeUsers []*v1.NifiUser,
	entity *nigoapi.AccessPolicyEntity) {
	// Add new user from the access policy
	for _, user := range addUsers {
		addUserToAccessPolicyEntity(user, entity)
	}

	// Remove user from the access policy
	var usersAccessPolicy []nigoapi.TenantEntity
	for _, user := range entity.Component.Users {
		contains := false

		for _, toRemove := range removeUsers {
			if user.Id == toRemove.Status.Id {
				contains = true
				break
			}
		}

		if !contains {
			usersAccessPolicy = append(usersAccessPolicy, user)
		}
	}
	entity.Component.Users = usersAccessPolicy
}

func WithManagedGroupsForPolicy(
	accessPolicy *v1.AccessPolicy,
	userGroups []*v1.NifiUserGroup,
	managedUserGroups ManagedUserGroups) []*v1.NifiUserGroup {
	userGroups = appendManagedGroup(userGroups, managedUserGroups.Nodes, RequiresManagedNodes(accessPolicy, managedUserGroups.Nodes))

	for _, group := range accessPolicy.IncludeManagedGroups {
		switch group {
		case v1.ManagedNodesAccessPolicyGroup:
			userGroups = appendManagedGroup(userGroups, managedUserGroups.Nodes, true)
		case v1.ManagedAdminsAccessPolicyGroup:
			userGroups = appendManagedGroup(userGroups, managedUserGroups.Admins, true)
		case v1.ManagedReadersAccessPolicyGroup:
			userGroups = appendManagedGroup(userGroups, managedUserGroups.Readers, true)
		}
	}

	return userGroups
}

func RequiresManagedGroups(accessPolicy *v1.AccessPolicy, managedUserGroups ManagedUserGroups) bool {
	return len(WithManagedGroupsForPolicy(accessPolicy, []*v1.NifiUserGroup{}, managedUserGroups)) > 0
}

func appendManagedGroup(userGroups []*v1.NifiUserGroup, managedGroup *v1.NifiUserGroup, shouldAppend bool) []*v1.NifiUserGroup {
	if !shouldAppend || managedGroup == nil || managedGroup.Status.Id == "" {
		return userGroups
	}
	for _, userGroup := range userGroups {
		if userGroupKey(userGroup) == userGroupKey(managedGroup) {
			return userGroups
		}
	}

	return append(userGroups, managedGroup)
}

func RequiresManagedNodes(accessPolicy *v1.AccessPolicy, managedNodesUserGroup *v1.NifiUserGroup) bool {
	if managedNodesUserGroup == nil || managedNodesUserGroup.Status.Id == "" {
		return false
	}

	return accessPolicy.Type == v1.ComponentAccessPolicyType &&
		accessPolicy.Resource == v1.DataAccessPolicyResource &&
		(accessPolicy.Action == v1.ReadAccessPolicyAction || accessPolicy.Action == v1.WriteAccessPolicyAction)
}

func ManagedNodesShouldKeepDataPolicy(userGroup, managedNodesUserGroup *v1.NifiUserGroup, action, resource string) bool {
	if userGroup == nil || managedNodesUserGroup == nil {
		return false
	}
	if userGroupKey(userGroup) != userGroupKey(managedNodesUserGroup) {
		return false
	}

	return (action == string(v1.ReadAccessPolicyAction) || action == string(v1.WriteAccessPolicyAction)) &&
		strings.HasPrefix(resource, string(v1.DataAccessPolicyResource)+"/")
}

func ManagedGroupShouldKeepPolicy(userGroup *v1.NifiUserGroup, managedUserGroups ManagedUserGroups,
	action, resource, rootProcessGroupId string, includeManagedGroupPolicies []v1.AccessPolicy) bool {
	if ManagedNodesShouldKeepDataPolicy(userGroup, managedUserGroups.Nodes, action, resource) {
		return true
	}

	for _, accessPolicy := range includeManagedGroupPolicies {
		if action != string(accessPolicy.Action) || resource != accessPolicy.GetResource(rootProcessGroupId) {
			continue
		}
		if accessPolicyIncludesManagedUserGroup(&accessPolicy, userGroup, managedUserGroups) {
			return true
		}
	}

	return false
}

func accessPolicyIncludesManagedUserGroup(accessPolicy *v1.AccessPolicy, userGroup *v1.NifiUserGroup,
	managedUserGroups ManagedUserGroups) bool {
	for _, group := range accessPolicy.IncludeManagedGroups {
		switch group {
		case v1.ManagedNodesAccessPolicyGroup:
			if userGroupKey(userGroup) == userGroupKey(managedUserGroups.Nodes) {
				return true
			}
		case v1.ManagedAdminsAccessPolicyGroup:
			if userGroupKey(userGroup) == userGroupKey(managedUserGroups.Admins) {
				return true
			}
		case v1.ManagedReadersAccessPolicyGroup:
			if userGroupKey(userGroup) == userGroupKey(managedUserGroups.Readers) {
				return true
			}
		}
	}
	return false
}

func addUserGroupToAccessPolicyEntity(userGroup *v1.NifiUserGroup, entity *nigoapi.AccessPolicyEntity) {
	if userGroup == nil || userGroup.Status.Id == "" {
		return
	}
	for _, existing := range entity.Component.UserGroups {
		if existing.Id == userGroup.Status.Id {
			return
		}
	}
	entity.Component.UserGroups = append(entity.Component.UserGroups, nigoapi.TenantEntity{Id: userGroup.Status.Id})
}

func addUserToAccessPolicyEntity(user *v1.NifiUser, entity *nigoapi.AccessPolicyEntity) {
	if user == nil || user.Status.Id == "" {
		return
	}
	for _, existing := range entity.Component.Users {
		if existing.Id == user.Status.Id {
			return
		}
	}
	entity.Component.Users = append(entity.Component.Users, nigoapi.TenantEntity{Id: user.Status.Id})
}

func userGroupKey(userGroup *v1.NifiUserGroup) types.NamespacedName {
	if userGroup == nil {
		return types.NamespacedName{}
	}
	return types.NamespacedName{Name: userGroup.Name, Namespace: userGroup.Namespace}
}
