# OpenShift wrapper provider

This provider is a wrapper for the Cluster API (`clusterapi`) provider to help
in mediating the migration from Machine API to Cluster API. The purpose of this
provider is to differentiate when authoritative resources are Machine API or
Cluster API in an OpenShift cluster. The Machine API logic is isolated to this
provider, while the Cluster API provider no longer contains Machine API specific
changes.

By default, this provider will enable both the OpenShift and ClusterAPI providers.
This allows it to process both MachineAPI and ClusterAPI resources. If you need
to explicitly disable the ClusterAPI provider, you should set the environment
variable `OPENSHIFT_CLUSTERAPI_DISABLE` to any non-empty value, eg `true`.
