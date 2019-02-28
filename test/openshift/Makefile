.PHONY: deps
deps:
	dep ensure -v

.PHONY: test-e2e
test-e2e: ## Run openshift specific e2e test
	go test -timeout 60m \
		-v ./vendor/github.com/openshift/cluster-api-actuator-pkg/pkg/e2e \
		-kubeconfig $${KUBECONFIG:-~/.kube/config} \
		-machine-api-namespace $${NAMESPACE:-openshift-machine-api} \
		-ginkgo.v \
		-args -v 5 -logtostderr

