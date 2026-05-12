package service

import (
	"github.com/fussraider/PopuGate/internal/model"
	"github.com/fussraider/PopuGate/pkg/telemt"
)

// BuildLinksForSecret generates proxy links for a secret across all accessible instances × domains.
func BuildLinksForSecret(sec *model.Secret, instances []model.Instance, serverIP string) []model.ProxyLink {
	secretTags := sec.GetTags()
	var links []model.ProxyLink
	for _, inst := range instances {
		if !inst.Enabled {
			continue
		}
		if !model.TagsMatch(inst.GetTags(), secretTags) {
			continue
		}
		for _, domain := range inst.AllDomains() {
			fullSecret := telemt.BuildFakeTLSSecret(sec.SecretKey, domain, inst.FakeTLS)
			links = append(links, model.ProxyLink{
				InstanceLabel: inst.Label,
				InstancePort:  inst.Port,
				Domain:        domain,
				TGLink:        telemt.BuildProxyLink(serverIP, inst.Port, fullSecret),
				WebLink:       telemt.BuildWebLink(serverIP, inst.Port, fullSecret),
			})
		}
	}
	return links
}
