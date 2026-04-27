package controller

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	runtimeClient "sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/konpyutaika/nifikop/api/v1"
	"github.com/konpyutaika/nifikop/pkg/clientwrappers/accesspolicies"
	"github.com/konpyutaika/nifikop/pkg/k8sutil"
)

func lookupManagedUserGroups(client runtimeClient.Client, cluster *v1.NifiCluster) (accesspolicies.ManagedUserGroups, error) {
	nodes, err := lookupManagedUserGroup(client, cluster, "managed-nodes")
	if err != nil {
		return accesspolicies.ManagedUserGroups{}, err
	}
	admins, err := lookupManagedUserGroup(client, cluster, "managed-admins")
	if err != nil {
		return accesspolicies.ManagedUserGroups{}, err
	}
	readers, err := lookupManagedUserGroup(client, cluster, "managed-readers")
	if err != nil {
		return accesspolicies.ManagedUserGroups{}, err
	}

	return accesspolicies.ManagedUserGroups{
		Nodes:   nodes,
		Admins:  admins,
		Readers: readers,
	}, nil
}

func lookupManagedUserGroup(client runtimeClient.Client, cluster *v1.NifiCluster, suffix string) (*v1.NifiUserGroup, error) {
	userGroup, err := k8sutil.LookupNifiUserGroup(client, fmt.Sprintf("%s.%s", cluster.Name, suffix), cluster.Namespace)
	if apierrors.IsNotFound(err) {
		return nil, nil
	}
	return userGroup, err
}

func (r *NifiUserGroupReconciler) includeManagedGroupAccessPolicies(
	ctx context.Context,
	cluster *v1.NifiCluster) ([]v1.AccessPolicy, error) {
	var accessPolicies []v1.AccessPolicy

	userGroups := &v1.NifiUserGroupList{}
	if err := r.Client.List(ctx, userGroups, runtimeClient.InNamespace(cluster.Namespace)); err != nil {
		return nil, err
	}
	for _, userGroup := range userGroups.Items {
		if !referencesCluster(userGroup.Spec.ClusterRef, cluster) {
			continue
		}
		accessPolicies = appendAccessPoliciesIncludingManagedGroups(accessPolicies, userGroup.Spec.AccessPolicies)
	}

	users := &v1.NifiUserList{}
	if err := r.Client.List(ctx, users, runtimeClient.InNamespace(cluster.Namespace)); err != nil {
		return nil, err
	}
	for _, user := range users.Items {
		if !referencesCluster(user.Spec.ClusterRef, cluster) {
			continue
		}
		accessPolicies = appendAccessPoliciesIncludingManagedGroups(accessPolicies, user.Spec.AccessPolicies)
	}

	return accessPolicies, nil
}

func referencesCluster(clusterRef v1.ClusterReference, cluster *v1.NifiCluster) bool {
	return clusterRef.Name == cluster.Name &&
		(clusterRef.Namespace == "" || clusterRef.Namespace == cluster.Namespace)
}

func appendAccessPoliciesIncludingManagedGroups(accessPolicies []v1.AccessPolicy, candidates []v1.AccessPolicy) []v1.AccessPolicy {
	for _, accessPolicy := range candidates {
		if len(accessPolicy.IncludeManagedGroups) == 0 {
			continue
		}
		accessPolicies = append(accessPolicies, accessPolicy)
	}
	return accessPolicies
}
