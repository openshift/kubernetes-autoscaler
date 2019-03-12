.PHONY: dep deps

dep:
	type -P dep 2>&1 > /dev/null || go get -u github.com/golang/dep/cmd/dep

deps: dep
	dep ensure -v

define test =
	go test -timeout 60m \
		-v ./vendor/github.com/openshift/cluster-api-actuator-pkg/pkg/e2e \
		-kubeconfig $${KUBECONFIG:-~/.kube/config} \
		-machine-api-namespace $${NAMESPACE:-openshift-machine-api} \
		-args -v 5 -logtostderr \
		$1 $2 $3 $4 $5 $6 $7 $8 $9
endef

.PHONY: test-e2e
test-e2e: ## Run openshift specific e2e test
	time $(call test,-ginkgo.v,-ginkgo.noColor=true)

.PHONY: test-e2e-autoscaler
test-e2e-autoscaler: ## Run autoscaler focused tests only
	time $(call test,-ginkgo.v,-ginkgo.focus=Autoscaler,-ginkgo.noColor=true)

.PHONY: revendor-test-e2e
revendor-test-e2e: dep
	curl --silent -o e2e_test.go https://raw.githubusercontent.com/openshift/cluster-api-actuator-pkg/master/pkg/e2e/e2e_test.go
	dep ensure -update github.com/openshift/cluster-api-actuator-pkg
	dep ensure -v
	$(RM) e2e_test.go
