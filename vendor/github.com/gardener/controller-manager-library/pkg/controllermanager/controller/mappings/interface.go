/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 *
 */

package mappings

import (
	"github.com/gardener/controller-manager-library/pkg/controllermanager/extension/mappings"
)

const TYPE_GROUP = mappings.TYPE_GROUP
const TYPE_CONTROLLER = "controller"

const CLUSTER_MAIN = mappings.CLUSTER_MAIN

type Definition = mappings.Definition
type Definitions = mappings.Definitions
type Registry = mappings.Registry
type Registerable = mappings.Registerable
type RegistrationInterface = mappings.RegistrationInterface

var NewRegistry = mappings.NewRegistry

var ClusterName = mappings.ClusterName
var DetermineClusterMappings = mappings.DetermineClusterMappings
var DetermineClusters = mappings.DetermineClusters
var MapClusters = mappings.MapClusters
var MapCluster = mappings.MapCluster
