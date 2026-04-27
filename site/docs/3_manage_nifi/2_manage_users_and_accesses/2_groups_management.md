---
id: 2_groups_management
title: Groups management
sidebar_label: Groups management
---

To simplify the access management Apache NiFi allows to define groups containing a list of users, on which we apply a list of access policies.
This part is supported by the operator using the `NifiUserGroup` resource:


```yaml
apiVersion: nifi.konpyutaika.com/v1
kind: NifiUserGroup
metadata:
  name: group-test
spec:
  # Contains the reference to the NifiCluster with the one the registry client is linked.
  clusterRef:
    name: nc
    namespace: nifikop
  # contains the list of reference to NifiUsers that are part to the group.
  usersRef:
    - name: nc-0-node.nc-headless.nifikop.svc.cluster.local
#     namespace: nifikop
    - name: nc-controller.nifikop.mgt.cluster.local
  # defines the list of access policies that will be granted to the group.
  accessPolicies:
      # defines the kind of access policy, could be "global" or "component".
    - type: global
      # defines the kind of action that will be granted, could be "read" or "write"
      action: read
      # resource defines the kind of resource targeted by this access policies, please refer to the following page:
      #	https://nifi.apache.org/docs/nifi-docs/html/administration-guide.html#access-policies
      resource: /counters
#      # componentType is used if the type is "component", it's allow to define the kind of component on which is the
#      # access policy
#      componentType: "process-groups"
#      # componentId is used if the type is "component", it's allow to define the id of the component on which is the
#      # access policy
#      componentId: ""
```

When you create a `NifiUserGroup` resource, the operator will create and manage a group named `${resource namespace}-${resource name}` in Nifi.
To declare the users that are part of this group, you just have to declare them in the [NifiUserGroup.UsersRef](../../5_references/6_nifi_usergroup#userreference) field.

:::important
The [NifiUserGroup.UsersRef](../../5_references/6_nifi_usergroup#userreference) requires to declare the name and namespace of a `NifiUser` resource, so it is previously required to declare the resource.

It's required to create the resource even if the user is already declared in NiFi Cluster (In that case the operator will just sync the kubernetes resource).
:::

Like for `NifiUser` you can declare a list of [AccessPolicies](../../5_references/2_nifi_user#accesspolicy) to give a list of access to your user.

In the example above we are giving to users `nc-0-node.nc-headless.nifikop.svc.cluster.local` and `nc-controller.nifikop.mgt.cluster.local` the right to view the counters information.

## Preserving managed access on restricted component policies

When you define a component-level policy on a child process group, NiFi treats that policy as an override of the inherited policy from the root process group. If the child policy targets `/data`, this can remove inherited access for operator-managed groups such as managed admins and managed readers.

NiFiKop automatically includes the managed nodes group on component `/data` read and write policies when it exists. This is required by Apache NiFi in clustered deployments so node-to-node request replication can list or delete queued FlowFiles.

Managed admins and managed readers are more sensitive because `/data` grants access to FlowFile metadata and content. Add them explicitly with `includeManagedGroups` when they should retain access to a restricted child policy:

```yaml
apiVersion: nifi.konpyutaika.com/v1
kind: NifiUserGroup
metadata:
  name: restricted-process-group-users
spec:
  clusterRef:
    name: nc
    namespace: nifikop
  usersRef:
    - name: restricted-user
      namespace: nifikop
  accessPolicies:
    - type: component
      componentType: process-groups
      componentId: <process-group-id>
      resource: /data
      action: read
      includeManagedGroups:
        - admins
        - readers
    - type: component
      componentType: process-groups
      componentId: <process-group-id>
      resource: /data
      action: write
      includeManagedGroups:
        - admins
```

Supported values are `nodes`, `admins`, and `readers`. `nodes` may be omitted for component `/data` read and write policies because NiFiKop adds it automatically for queue operations.
