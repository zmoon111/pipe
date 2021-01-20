// Copyright 2021 The PipeCD Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package lambda

import (
	"context"

	"github.com/pipe-cd/pipe/pkg/app/piped/executor"
	"github.com/pipe-cd/pipe/pkg/model"
)

type rollbackExecutor struct {
	executor.Input
}

func (e *rollbackExecutor) Execute(sig executor.StopSignal) model.StageStatus {
	var (
		ctx            = sig.Context()
		originalStatus = e.Stage.Status
		status         model.StageStatus
	)

	switch model.Stage(e.Stage.Name) {
	case model.StageRollback:
		status = e.ensureRollback(ctx)
	default:
		e.LogPersister.Errorf("Unsupported stage %s for cloudrun application", e.Stage.Name)
		return model.StageStatus_STAGE_FAILURE
	}

	return executor.DetermineStageStatus(sig.Signal(), originalStatus, status)
}

func (e *rollbackExecutor) ensureRollback(ctx context.Context) model.StageStatus {
	// Not rollback in case this is the first deployment.
	if e.Deployment.RunningCommitHash == "" {
		e.LogPersister.Errorf("Unable to determine the last deployed commit to rollback. It seems this is the first deployment.")
		return model.StageStatus_STAGE_FAILURE
	}

	runningDS, err := e.RunningDSP.GetReadOnly(ctx, e.LogPersister)
	if err != nil {
		e.LogPersister.Errorf("Failed to prepare running deploy source data (%v)", err)
		return model.StageStatus_STAGE_FAILURE
	}

	deployCfg := runningDS.DeploymentConfig.LambdaDeploymentSpec
	if deployCfg == nil {
		e.LogPersister.Errorf("Malformed deployment configuration: missing LambdaDeploymentSpec")
		return model.StageStatus_STAGE_FAILURE
	}

	cloudProviderName, cloudProviderCfg, found := findCloudProvider(&e.Input)
	if !found {
		return model.StageStatus_STAGE_FAILURE
	}

	fm, ok := loadFunctionManifest(&e.Input, deployCfg.Input.FunctionManifestFile, runningDS)
	if !ok {
		return model.StageStatus_STAGE_FAILURE
	}

	if !sync(ctx, &e.Input, cloudProviderName, cloudProviderCfg, fm) {
		return model.StageStatus_STAGE_FAILURE
	}

	return model.StageStatus_STAGE_FAILURE
}