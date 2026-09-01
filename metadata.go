package config

type Metadata struct {
	TemplateFile       string
	OverlayFile        string
	PlaceholderSources []string
	OverrideSources    []string
}

func (metadata Metadata) clone() Metadata {
	metadata.PlaceholderSources = append([]string(nil), metadata.PlaceholderSources...)
	metadata.OverrideSources = append([]string(nil), metadata.OverrideSources...)
	return metadata
}
