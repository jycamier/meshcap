module github.com/jycamier/meshcap/wasm

go 1.24.9

require (
	github.com/jycamier/meshcap/pkg/meshcap v0.0.0
	github.com/proxy-wasm/proxy-wasm-go-sdk v0.0.0-20260105142703-44c7d5847745
)

replace github.com/jycamier/meshcap/pkg/meshcap => ../pkg/meshcap
