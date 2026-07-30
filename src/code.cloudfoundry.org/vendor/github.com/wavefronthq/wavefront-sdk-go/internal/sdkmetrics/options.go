package sdkmetrics

type RegistryOption func(*realRegistry)

func SetSource(source string) RegistryOption {
	return func(registry *realRegistry) {
		registry.source = source
	}
}

func SetTags(tags map[string]string) RegistryOption {
	return func(registry *realRegistry) {
		registry.tags = tags
	}
}

func SetTag(key, value string) RegistryOption {
	return func(registry *realRegistry) {
		if registry.tags == nil {
			registry.tags = make(map[string]string)
		}
		registry.tags[key] = value
	}
}

func SetPrefix(prefix string) RegistryOption {
	return func(registry *realRegistry) {
		registry.prefix = prefix
	}
}
