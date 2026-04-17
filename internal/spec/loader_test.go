package spec

import (
	"testing"

	"github.com/Pantani/gorchestrator/internal/api/v1alpha1"
)

func TestApplyDefaultsSelectsPluginFromFamilyAliases(t *testing.T) {
	tests := []struct {
		name       string
		family     string
		wantPlugin string
	}{
		{name: "generic", family: "generic", wantPlugin: "generic-process"},
		{name: "cometbft", family: "cometbft", wantPlugin: "cometbft-family"},
		{name: "evm", family: "evm", wantPlugin: "evm-family"},
		{name: "ethereum alias", family: "ethereum", wantPlugin: "evm-family"},
		{name: "solana", family: "solana", wantPlugin: "solana-family"},
		{name: "bitcoin", family: "bitcoin", wantPlugin: "bitcoin-family"},
		{name: "btc alias", family: "btc", wantPlugin: "bitcoin-family"},
		{name: "cosmos", family: "cosmos", wantPlugin: "cosmos-family"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			cluster := &v1alpha1.ChainCluster{
				Spec: v1alpha1.ChainClusterSpec{
					Family: tc.family,
					Runtime: v1alpha1.RuntimeSpec{
						Backend: "docker-compose",
					},
					NodePools: []v1alpha1.NodePoolSpec{
						{
							Name: "nodes",
							Template: v1alpha1.NodeSpec{
								Workloads: []v1alpha1.WorkloadSpec{
									{Name: "node", Image: "alpine:3.20"},
								},
							},
						},
					},
				},
			}
			ApplyDefaults(cluster)
			if cluster.Spec.Plugin != tc.wantPlugin {
				t.Fatalf("expected plugin %q, got %q", tc.wantPlugin, cluster.Spec.Plugin)
			}
		})
	}
}

func TestApplyDefaultsKeepsExplicitPlugin(t *testing.T) {
	cluster := &v1alpha1.ChainCluster{
		Spec: v1alpha1.ChainClusterSpec{
			Family: "evm",
			Plugin: "custom-plugin",
			Runtime: v1alpha1.RuntimeSpec{
				Backend: "docker-compose",
			},
			NodePools: []v1alpha1.NodePoolSpec{
				{
					Name: "nodes",
					Template: v1alpha1.NodeSpec{
						Workloads: []v1alpha1.WorkloadSpec{
							{Name: "node", Image: "alpine:3.20"},
						},
					},
				},
			},
		},
	}
	ApplyDefaults(cluster)
	if cluster.Spec.Plugin != "custom-plugin" {
		t.Fatalf("expected explicit plugin to be preserved, got %q", cluster.Spec.Plugin)
	}
}

func TestApplyDefaultsNormalizesRestartPolicy(t *testing.T) {
	cluster := &v1alpha1.ChainCluster{
		Spec: v1alpha1.ChainClusterSpec{
			Family: "generic",
			Runtime: v1alpha1.RuntimeSpec{
				Backend: "docker-compose",
			},
			NodePools: []v1alpha1.NodePoolSpec{
				{
					Name: "nodes",
					Template: v1alpha1.NodeSpec{
						Workloads: []v1alpha1.WorkloadSpec{
							{Name: "node", Image: "alpine:3.20", RestartPolicy: "ALWAYS"},
						},
					},
				},
			},
		},
	}

	ApplyDefaults(cluster)
	got := cluster.Spec.NodePools[0].Template.Workloads[0].RestartPolicy
	if got != v1alpha1.RestartAlways {
		t.Fatalf("expected restartPolicy %q, got %q", v1alpha1.RestartAlways, got)
	}
}
