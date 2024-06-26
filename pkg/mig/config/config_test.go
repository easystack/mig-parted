/*
 * Copyright (c) 2021, NVIDIA CORPORATION.  All rights reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package config

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock/dgxa100"

	"github.com/NVIDIA/mig-parted/internal/nvlib"
	"github.com/NVIDIA/mig-parted/pkg/types"
)

func NewMockLunaServerMigConfigManager() Manager {
	nvml := dgxa100.New()
	nvlib := nvlib.NewMock(nvml)
	return &nvmlMigConfigManager{nvml, nvlib}
}

func EnableMigMode(manager Manager, gpu int) (nvml.Return, nvml.Return) {
	m := manager.(*nvmlMigConfigManager)
	n := m.nvml.(*dgxa100.Server)
	r1, r2 := n.Devices[gpu].SetMigMode(nvml.DEVICE_MIG_ENABLE)
	return r1, r2
}

func TestGetSetMigConfig(t *testing.T) {
	types.SetMockNVdevlib()
	mcg := NewA100_SXM4_40GB_MigConfigGroup()

	type testCase struct {
		description string
		config      types.MigConfig
	}
	testCases := func() []testCase {
		var testCases []testCase
		for _, mc := range mcg.GetPossibleConfigurations() {
			tc := testCase{
				fmt.Sprintf("%v", mc.Flatten()),
				mc,
			}
			testCases = append(testCases, tc)
		}
		return testCases
	}()

	for i := range testCases {
		tc := testCases[i] // to allow us to run parallelly
		t.Run(tc.description, func(t *testing.T) {
			t.Parallel()

			nvmlLib := dgxa100.New()
			manager := NewMockLunaServerMigConfigManager()

			numGPUs, ret := nvmlLib.DeviceGetCount()
			require.NotNil(t, ret, "Unexpected nil return from DeviceGetCount")
			require.Equal(t, ret, nvml.SUCCESS, "Unexpected return value from DeviceGetCount")

			for i := 0; i < numGPUs; i++ {
				r1, r2 := EnableMigMode(manager, i)
				require.Equal(t, nvml.SUCCESS, r1)
				require.Equal(t, nvml.SUCCESS, r2)

				err := manager.SetMigConfig(i, tc.config)
				require.Nil(t, err, "Unexpected failure from SetMigConfig")

				config, err := manager.GetMigConfig(i)
				require.Nil(t, err, "Unexpected failure from GetMigConfig")
				require.Equal(t, tc.config.Flatten(), config.Flatten(), "Retrieved MigConfig different than what was set")
			}
		})
	}
}

func TestClearMigConfig(t *testing.T) {
	types.SetMockNVdevlib()
	mcg := NewA100_SXM4_40GB_MigConfigGroup()

	type testCase struct {
		description string
		config      types.MigConfig
	}
	testCases := func() []testCase {
		var testCases []testCase
		for _, mc := range mcg.GetPossibleConfigurations() {
			tc := testCase{
				fmt.Sprintf("%v", mc.Flatten()),
				mc,
			}
			testCases = append(testCases, tc)
		}
		return testCases
	}()

	for i := range testCases {
		tc := testCases[i] // to allow us to run parallelly
		t.Run(tc.description, func(t *testing.T) {
			t.Parallel()

			manager := NewMockLunaServerMigConfigManager()

			r1, r2 := EnableMigMode(manager, 0)
			require.Equal(t, nvml.SUCCESS, r1)
			require.Equal(t, nvml.SUCCESS, r2)

			err := manager.SetMigConfig(0, tc.config)
			require.Nil(t, err, "Unexpected failure from SetMigConfig")

			err = manager.ClearMigConfig(0)
			require.Nil(t, err, "Unexpected failure from ClearMigConfig")

			config, err := manager.GetMigConfig(0)
			require.Nil(t, err, "Unexpected failure from GetMigConfig")
			require.Equal(t, 0, len(config.Flatten()), "Unexpected number of configured MIG profiles")
		})
	}
}

func TestIteratePermutationsUntilSuccess(t *testing.T) {
	factorial := func(n int) int {
		product := 1
		for i := 1; i <= n; i++ {
			product *= i
		}
		return product
	}

	uniquePermutations := func(mc types.MigConfig) int {
		perms := factorial(len(mc.Flatten()))
		for _, v := range mc {
			perms /= factorial(v)
		}
		return perms
	}

	mcg := NewA100_SXM4_40GB_MigConfigGroup()

	type testCase struct {
		description  string
		config       types.MigConfig
		successAfter int
	}
	testCases := func() []testCase {
		var testCases []testCase
		for _, mc := range mcg.GetPossibleConfigurations() {
			successAfter := rand.Intn(uniquePermutations(mc)) + 1
			tc := testCase{
				fmt.Sprintf("%v:%v", mc.Flatten(), successAfter),
				mc,
				successAfter, // Random stop between 1 and uniquePermutations
			}
			testCases = append(testCases, tc)
		}
		for _, mc := range mcg.GetPossibleConfigurations() {
			tc := testCase{
				fmt.Sprintf("%v:%v", mc.Flatten(), -1),
				mc,
				-1, // Never stop, so expect failure after uniquePermutations
			}
			testCases = append(testCases, tc)
		}
		return testCases
	}()

	for i := range testCases {
		tc := testCases[i] // to allow us to run parallelly
		t.Run(tc.description, func(t *testing.T) {
			t.Parallel()

			iteration := 0
			err := iteratePermutationsUntilSuccess(tc.config, func(perm []*types.MigProfile) error {
				iteration++
				if iteration == tc.successAfter {
					return nil
				}
				err := fmt.Errorf("Failed iteration: %v", iteration)
				return err
			})
			if err == nil {
				require.Equal(t, tc.successAfter, iteration, "Success on wrong iteration")
			} else {
				require.Equal(t, uniquePermutations(tc.config), iteration, "Failed after wrong number of iterations")
			}
		})
	}
}
