package manifests

import (
	"path/filepath"
	"sort"

	"github.com/ghodss/yaml"

	clusterdnsopapi "github.com/openshift/cluster-dns-operator/pkg/apis/dns/v1alpha1"
	clusterdnsopmanifests "github.com/openshift/cluster-dns-operator/pkg/manifests"

	"github.com/openshift/installer/pkg/asset"
	"github.com/openshift/installer/pkg/asset/installconfig"
	"github.com/openshift/installer/pkg/types"
)

// clusterDNSOperator generates the cluster-dns-operator-*.yml files
type clusterDNSOperator struct {
	installConfigAsset asset.Asset
	installConfig      *types.InstallConfig
}

var _ asset.Asset = (*clusterDNSOperator)(nil)

// Name returns a human friendly name for the operator
func (cdo *clusterDNSOperator) Name() string {
	return "Cluster DNS Operator"
}

// Dependencies returns all of the dependencies directly needed by an
// clusterDNSOperator asset.
func (cdo *clusterDNSOperator) Dependencies() []asset.Asset {
	return []asset.Asset{
		cdo.installConfigAsset,
	}
}

// Generate generates the cluster-dns-operator-*.yml files
func (cdo *clusterDNSOperator) Generate(dependencies map[asset.Asset]*asset.State) (*asset.State, error) {
	ic, err := installconfig.GetInstallConfig(cdo.installConfigAsset, dependencies)
	if err != nil {
		return nil, err
	}
	cdo.installConfig = ic

	// installconfig is ready, we can create the core config from it now
	dnsConfig, err := cdo.dnsConfig()
	if err != nil {
		return nil, err
	}

	assetData, err := cdo.assetData()
	if err != nil {
		return nil, err
	}

	keys := make([]string, 0)
	for k := range assetData {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	assetContents := make([]asset.Content, 0)
	for _, k := range keys {
		assetContents = append(assetContents, asset.Content{
			Name: filepath.Join("cluster-dns-operator", k),
			Data: assetData[k],
		})
	}

	assetContents = append(assetContents, asset.Content{
		Name: "cluster-dns-operator-config.yml",
		Data: dnsConfig,
	})

	return &asset.State{Contents: assetContents}, nil
}

func (cdo *clusterDNSOperator) dnsOperatorConfig() (*clusterdnsopapi.ClusterDNS, error) {
	clusterIP, err := installconfig.ClusterDNSIP(cdo.installConfig)
	if err != nil {
		return nil, err
	}

	return &clusterdnsopapi.ClusterDNS{
		Spec: clusterdnsopapi.ClusterDNSSpec{
			// Check if BaseDomain is correct?
			ClusterIP:     &clusterIP,
			ClusterDomain: &cdo.installConfig.BaseDomain,
		},
	}, nil
}

func (cdo *clusterDNSOperator) dnsConfig() ([]byte, error) {
	return yaml.Marshal(cdo.dnsOperatorConfig())
}

func (cdo *clusterDNSOperator) assetData() (map[string][]byte, error) {
	f := clusterdnsopmanifests.NewFactory()
	return f.OperatorAssetContent()
}
