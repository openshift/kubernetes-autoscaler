# OpenShift wrapper provider

This provider is a wrapper for the Cluster API (`clusterapi`) provider to help
in mediating the migration from Machine API to Cluster API. The purpose of this
provider is to differentiate when authoritative resources are Machine API or
Cluster API in an OpenShift cluster. The Machine API logic is isolated to this
provider, while the Cluster API provider no longer contains Machine API specific
changes.
