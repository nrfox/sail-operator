package main

import (
	"fmt"

	_ "github.com/istio-ecosystem/sail-operator/chart/plugins/sail-operator-postrender/initfix"

	"github.com/extism/go-pdk"
)

type PluginInput struct {
	Manifests string   `json:"manifests"`
	ExtraArgs []string `json:"extraArgs"`
}

type PluginOutput struct {
	Manifests string `json:"manifests"`
}

//go:wasmexport helm_plugin_main
func helmPluginMain() int32 {
	pdk.Log(pdk.LogInfo, "running sail-operator-postrender plugin")
	// var input PluginInput
	// if err := pdk.InputJSON(&input); err != nil {
	// 	pdk.SetError(err)
	// 	return 1
	// }
	input := string(pdk.Input())
	pdk.SetError(fmt.Errorf("input: %#v\n test error", input))
	pdk.SetError(fmt.Errorf("input string: %#v\n test error", pdk.InputString()))
	return 0

	// v, meta, err := ParseValuesFromManifests([]byte(input.Manifests))
	// if err != nil {
	// 	pdk.SetError(err)
	// 	return 1
	// }

	// output, err := BuildAllResources(v, meta)
	// if err != nil {
	// 	pdk.SetError(err)
	// 	return 1
	// }

	// if err := pdk.OutputJSON(PluginOutput{Manifests: string(output)}); err != nil {
	// 	pdk.SetError(err)
	// 	return 1
	// }

	// return 0
}

func main() {}
