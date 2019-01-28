# Cluster Autoscaler with Cluster API

## How to deploy CA with kubemark actuator

1. Deploy cluster with `NodeRestriction` plugin disabled and cluster API stack with kubemark actuator (you can follow instructions at https://github.com/openshift/cluster-api-provider-kubemark#how-to-deploy-and-test-the-machine-controller-with-minikube)

1. Deploy autoscaler:
   1. Build cluster autoscaler binary:
      ```sh
      $ go build -o bin/cluster-autoscaler k8s.io/autoscaler/cluster-autoscaler
      ```

   1. Run the cluster autoscaler binary:
      ```sh
      $ ./bin/cluster-autoscaler --kubeconfig ~/.kube/config --logtostderr --scan-interval=10s --cloud-provider=cluster-api --scale-down-delay-after-failure=10s --scale-down-unneeded-time=10s --scale-down-delay-after-add=10s --leader-elect=false 6 -v=4
      ```

1. Deploy machineset
   ```yaml
    apiVersion: cluster.k8s.io/v1alpha1
    kind: MachineSet
    metadata:
      name: kubemark-actuator-testing-machineset
      namespace: default
    annotations:
      sigs.k8s.io/cluster-api-autoscaler-node-group-min-size: "1"
      sigs.k8s.io/cluster-api-autoscaler-node-group-max-size: "12"
    spec:
      replicas: 1
      selector:
        matchLabels:
          sigs.k8s.io/cluster-api-machineset: test-kubemark
      template:
        metadata:
          labels:
            sigs.k8s.io/cluster-api-machineset: test-kubemark
        spec:
          metadata:
            labels:
              node-role.kubernetes.io/compute: ""
          providerSpec:
            value:
              apiVersion: kubemarkproviderconfig.k8s.io/v1alpha1
              kind: KubemarkMachineProviderConfig
          versions:
            kubelet: 1.10.1
            controlPlane: 1.10.1
   ```

1. Deploy workload (notice that all the workload pods are scheduled only to `compute` nodes to avoid scheduling pods to non-hollow nodes):
   ```yaml
    apiVersion: batch/v1
    kind: Job
    metadata:
      name: workload
      generateName: work-queue-
    spec:
      template:
        spec:
          containers:
          - name: work
            image: busybox
            command: ["sleep",  "120"]
            resources:
              requests:
                memory: 500Mi
                cpu: 500m
          restartPolicy: Never
          nodeSelector:
            node-role.kubernetes.io/compute: ""
          tolerations:
          - key: kubemark
            operator: Exists
      backoffLimit: 4
      completions: 40
      parallelism: 40
   ```

1. Observe how CA scales up and brings additional hollow nodes

## Observed output after scalling up

```sh
$ sudo kubectl get pods | grep kubemark
hollow-node-kubemark-actuator-testing-machineset-2pjg9   2/2       Running   0          5m
hollow-node-kubemark-actuator-testing-machineset-42kt6   2/2       Running   0          5m
hollow-node-kubemark-actuator-testing-machineset-9d7b5   2/2       Running   0          5m
hollow-node-kubemark-actuator-testing-machineset-clczb   2/2       Running   0          8m
hollow-node-kubemark-actuator-testing-machineset-cxzjh   2/2       Running   0          29m
hollow-node-kubemark-actuator-testing-machineset-g9lnz   2/2       Running   0          8m
hollow-node-kubemark-actuator-testing-machineset-h7r4s   2/2       Running   0          8m
hollow-node-kubemark-actuator-testing-machineset-m4np2   2/2       Running   0          8m
hollow-node-kubemark-actuator-testing-machineset-mwqjc   2/2       Running   0          5m
hollow-node-kubemark-actuator-testing-machineset-tfx49   2/2       Running   0          5m
hollow-node-kubemark-actuator-testing-machineset-z5zjm   2/2       Running   0          5m
hollow-node-kubemark-actuator-testing-machineset-zs48t   2/2       Running   0          8m
```

```sh
$ sudo kubectl get nodes
NAME                                                     STATUS    ROLES     AGE       VERSION
hollow-node-kubemark-actuator-testing-machineset-2pjg9   Ready     compute   5m        v1.11.3-1+9a66bfb457afde-dirty
hollow-node-kubemark-actuator-testing-machineset-42kt6   Ready     compute   5m        v1.11.3-1+9a66bfb457afde-dirty
hollow-node-kubemark-actuator-testing-machineset-9d7b5   Ready     compute   5m        v1.11.3-1+9a66bfb457afde-dirty
hollow-node-kubemark-actuator-testing-machineset-clczb   Ready     compute   8m        v1.11.3-1+9a66bfb457afde-dirty
hollow-node-kubemark-actuator-testing-machineset-cxzjh   Ready     compute   25m       v1.11.3-1+9a66bfb457afde-dirty
hollow-node-kubemark-actuator-testing-machineset-g9lnz   Ready     compute   8m        v1.11.3-1+9a66bfb457afde-dirty
hollow-node-kubemark-actuator-testing-machineset-h7r4s   Ready     compute   8m        v1.11.3-1+9a66bfb457afde-dirty
hollow-node-kubemark-actuator-testing-machineset-m4np2   Ready     compute   9m        v1.11.3-1+9a66bfb457afde-dirty
hollow-node-kubemark-actuator-testing-machineset-mwqjc   Ready     compute   5m        v1.11.3-1+9a66bfb457afde-dirty
hollow-node-kubemark-actuator-testing-machineset-tfx49   Ready     compute   5m        v1.11.3-1+9a66bfb457afde-dirty
hollow-node-kubemark-actuator-testing-machineset-z5zjm   Ready     compute   5m        v1.11.3-1+9a66bfb457afde-dirty
hollow-node-kubemark-actuator-testing-machineset-zs48t   Ready     compute   8m        v1.11.3-1+9a66bfb457afde-dirty
minikube                                                 Ready     master    1h        v1.11.3
```

## TODO

- use cluster autoscaler operator to deploy the CA instead or running the binary locally
- TBD
