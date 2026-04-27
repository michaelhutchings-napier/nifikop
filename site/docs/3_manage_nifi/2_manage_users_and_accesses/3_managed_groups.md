---
id: 3_managed_groups
title: Managed groups
sidebar_label: Managed groups
---

In some case these two features could be heavy to define, for example when you have 10 dataflows with one cluster for each of them, it will lead in a lot of `.yaml` files.
To simplify this, we implement in the operator 2 `managed groups`:

- **Admins:** a group giving access to everything on the NiFi Cluster,
- **Readers:** a group giving access as viewer on the NiFi Cluster.

You can directly define the list of users who belong to each of them in the `NifiCluster.Spec` field:

```yaml
apiVersion: nifi.konpyutaika.com/v1
kind: NifiCluster
metadata:
  name: mynifi
spec:
  ...
  oneNifiNodePerNode: false
  propagateLabels: true
  managedAdminUsers:
    -  identity: "alexandre.guitton@konpyutaika.com"
       name: "aguitton"
    -  identity: "nifiuser@konpyutaika.com"
       name: "nifiuser"
  managedReaderUsers:
    -  identity: "toto@konpyutaika.com"
       name: "toto"
    ...
```

In this example the operator will create and manage 3 `NifiUsers`:

- **aguitton**, with the identity: `alexandre.guitton@konpyutaika.com`
- **nifiuser**, with the identity: `nifiuser@konpyutaika.com`
- **toto**, with the identity: `toto@konpyutaika.com`

And create and manage two groups:

- **managed-admins:** that will contain 3 users (**aguitton**, **nifiuser**, **nc-controller.nifikop.mgt.cluster.local** which is the controller user).
- **managed-readers:** that will contain 1 user (**toto**)

And the rest of the stuff will be reconciled and managed as described for `NifiUsers` and `NifiUserGroups`.

When a user-defined access policy is created on a child component, NiFi breaks inheritance from the parent policy for that resource. For `/data` policies this means managed admins and managed readers do not automatically keep inherited access to the child process group.

NiFiKop always keeps managed nodes on component `/data` read and write policies when the managed nodes group exists, because Apache NiFi requires node identities on those policies for clustered queue listing and queue deletion. `managed-nodes` is not configured through `includeManagedGroups`.

Managed admins and managed readers must be included explicitly on component `/data` read/write policies, and should only be included when they intentionally need data access:

```yaml
accessPolicies:
  - type: component
    componentType: process-groups
    componentId: <process-group-id>
    resource: /data
    action: read
    includeManagedGroups:
      - admins
      - readers
```

This keeps `/data` access explicit while allowing restricted child policies to preserve the managed admin or reader groups that should still have access.

:::note
There is one more group that is created and managed by the operator, this is the **managed-nodes** group, for each node a `NifiUser` is created, and we automatically add them to this group to give them the right list of accesses.

To get the list of managed groups just check the list of `NifiUserGroup`:

```console
kubectl get -n nifikop nifiusergroups.nifi.konpyutaika.com 
NAME              AGE
managed-admins    6d7h
managed-nodes     6d7h
managed-readers   6d7h
```
:::
