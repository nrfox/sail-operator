VERSION = 3.0.0-tp.2
OPERATOR_NAME = servicemeshoperator3
HUB = quay.io/maistra-dev
CHANNELS = candidates
HELM_VALUES_FILE = ossm/values.yaml
VERSIONS_YAML_FILE ?= ossm/versions.yaml
GENERATE_RELATED_IMAGES = false