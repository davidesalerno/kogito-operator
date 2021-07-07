// Copyright 2019 Red Hat, Inc. and/or its affiliates
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package steps

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cucumber/godog"
	"github.com/kiegroup/kogito-operator/test/pkg/config"
	"github.com/kiegroup/kogito-operator/test/pkg/framework"
	"k8s.io/apimachinery/pkg/runtime"
)

var (
	// Map of created namespaces
	namespacesCreated sync.Map

	logsKubernetesObjects = []runtime.Object{}
)

// Data contains all data needed by Gherkin steps to run
type Data struct {
	Namespace              string
	StartTime              time.Time
	KogitoExamplesLocation string
	ScenarioName           string
	ScenarioContext        map[string]string
}

// RegisterAllSteps register all steps available to the test suite
func (data *Data) RegisterAllSteps(ctx *godog.ScenarioContext) {
	registerGitSteps(ctx, data)
	registerGrafanaSteps(ctx, data)
	registerGraphQLSteps(ctx, data)
	registerHTTPSteps(ctx, data)
	registerImageRegistrySteps(ctx, data)
	registerInfinispanSteps(ctx, data)
	registerKafkaSteps(ctx, data)
	registerKnativeSteps(ctx, data)
	registerKogitoBuildSteps(ctx, data)
	registerKogitoRuntimeSteps(ctx, data)
	registerKogitoDataIndexServiceSteps(ctx, data)
	registerKogitoExplainabilityServiceSteps(ctx, data)
	registerKogitoInfraSteps(ctx, data)
	registerKogitoJobsServiceSteps(ctx, data)
	registerKogitoManagementConsoleSteps(ctx, data)
	registerKogitoTaskConsoleSteps(ctx, data)
	registerKogitoTrustyServiceSteps(ctx, data)
	registerKogitoTrustyUISteps(ctx, data)
	registerKubernetesSteps(ctx, data)
	registerMavenSteps(ctx, data)
	registerMongoDBSteps(ctx, data)
	registerOpenShiftSteps(ctx, data)
	registerOperatorSteps(ctx, data)
	registerPrometheusSteps(ctx, data)
	registerProcessSteps(ctx, data)
	registerTaskSteps(ctx, data)
	registerKogitoDeployFilesSteps(ctx, data)
	registerKeycloakSteps(ctx, data)
}

// RegisterLogsKubernetesObjects allows to change which kubernetes objects logs should be saved
func (data *Data) RegisterLogsKubernetesObjects(objects ...runtime.Object) {
	logsKubernetesObjects = append(logsKubernetesObjects, objects...)
}

// BeforeScenario configure the data before a scenario is launched
func (data *Data) BeforeScenario(scenario *godog.Scenario) error {
	data.StartTime = time.Now()
	data.Namespace = getNamespaceName()
	data.KogitoExamplesLocation = createTemporaryFolder()
	data.ScenarioName = scenario.GetName()
	data.ScenarioContext = map[string]string{}

	var err error
	framework.GetLogger(data.Namespace).Info(fmt.Sprintf("Scenario %s", scenario.GetName()))
	go func() {
		err = framework.StartPodLogCollector(data.Namespace)
	}()
	if err != nil {
		return err
	}

	return nil
}

func getNamespaceName() string {
	if namespaceName := config.GetNamespaceName(); len(namespaceName) > 0 {
		return namespaceName
	}
	return generateNamespaceName()
}

func generateNamespaceName() string {
	ns := framework.GenerateNamespaceName("cucumber")
	for isNamespaceAlreadyCreated(ns) {
		ns = framework.GenerateNamespaceName("cucumber")
	}
	namespacesCreated.Store(ns, true)
	return ns
}

func isNamespaceAlreadyCreated(namespace string) bool {
	_, exists := namespacesCreated.Load(namespace)
	return exists
}

func createTemporaryFolder() string {
	dir, err := framework.CreateTemporaryFolder("kogito-examples")
	if err != nil {
		panic(fmt.Errorf("Error creating new temporary folder: %v", err))
	}
	return dir
}

// AfterScenario executes some actions on data after a scenario is finished
func (data *Data) AfterScenario(scenario *godog.Scenario, err error) error {
	error := framework.OperateOnNamespaceIfExists(data.Namespace, func(namespace string) error {
		if err := framework.StopPodLogCollector(namespace); err != nil {
			framework.GetMainLogger().Error(err, "Error stopping log collector", "namespace", namespace)
		}
		if err := framework.FlushLogger(namespace); err != nil {
			framework.GetMainLogger().Error(err, "Error flushing running logs", "namespace", namespace)
		}
		if err := framework.BumpEvents(data.Namespace); err != nil {
			framework.GetMainLogger().Error(err, "Error bumping events", "namespace", namespace)
		}
		if err := framework.LogKubernetesObjects(data.Namespace, logsKubernetesObjects...); err != nil {
			framework.GetMainLogger().Error(err, "Error logging Kubernetes objects", "namespace", namespace)
		}
		return nil
	})

	handleScenarioResult(data, scenario, err)
	logScenarioDuration(data)
	deleteTemporaryExamplesFolder(data)

	if error != nil {
		return error
	}

	return nil
}

// ResolveWithScenarioContext replaces all the variables in the string with their values.
func (data *Data) ResolveWithScenarioContext(str string) string {
	result := str
	for name, value := range data.ScenarioContext {
		result = strings.ReplaceAll(result, "{"+name+"}", value)
	}

	return result
}

func logScenarioDuration(data *Data) {
	endTime := time.Now()
	duration := endTime.Sub(data.StartTime)
	framework.GetLogger(data.Namespace).Info("Scenario duration", "duration", duration.String())
}

func handleScenarioResult(data *Data, scenario *godog.Scenario, err error) {
	newLogFolderName := fmt.Sprintf("%s - %s", strings.ReplaceAll(scenario.GetName(), "/", "_"), data.Namespace)
	var parentLogFolder string
	if err != nil {
		framework.GetLogger(data.Namespace).Error(err, "Error in scenario", "scenarioName", scenario.GetName())
		parentLogFolder = "error"
	} else {
		parentLogFolder = "success"
		framework.GetLogger(data.Namespace).Info("Successful scenario", "scenarioName", scenario.GetName())
	}
	err = framework.RenameLogFolder(data.Namespace, parentLogFolder, newLogFolderName)
	if err != nil {
		framework.GetMainLogger().Error(err, "Error while moving log foler", "logFolder", newLogFolderName, "namespace", data.Namespace)
	}
}

func deleteTemporaryExamplesFolder(data *Data) {
	err := framework.DeleteFolder(data.KogitoExamplesLocation)
	if err != nil {
		framework.GetMainLogger().Error(err, "Error while deleting temporary examples folder", "folderName", data.KogitoExamplesLocation)
	}
}
