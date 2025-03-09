// The package is a simple implementation which is mainly
// for "tmd fmt" and "tmd gen" commands.
//
// The APIs provided in the lib package is not concurrently safe.
package tmd

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"os"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

//go:embed tmd.wasm
var tmdWasm []byte

// All methods of Lib are not concurently safe.
type Lib struct {
	context context.Context

	runtime wazero.Runtime

	module api.Module
	memory api.Memory

	apiLibVersion   wasmApi
	apiBufferOffset wasmApi
	apiTmdToHtml    wasmApi
	apiTmdFormat    wasmApi

	inputTmdDataLength uint32
}

type wasmApi struct {
	name string
	lib  *Lib
	fun  api.Function
}

func makeWasmApi(lib *Lib, name string) wasmApi {
	return wasmApi{
		name: name,
		lib:  lib,
		fun:  lib.module.ExportedFunction(name),
	}
}

func (wa *wasmApi) call() (uint32, error) {
	rets, err := wa.fun.Call(wa.lib.context)
	if err != nil {
		return 0, err
	}
	if offset := int32(rets[0]); offset < 0 {
		return 0, fmt.Errorf("%s error: %s", wa.name, wa.lib.readCString(uint32(-offset-1)))
	}
	return uint32(rets[0]), nil
}

func printMessages(_ context.Context, m api.Module, offset, byteCount, offset2, byteCount2 uint32, extraInt32 int32) {
	buf, ok := m.Memory().Read(offset, byteCount)
	if !ok {
		log.Panicf("Memory.Read(%d, %d) out of range (1)", offset, byteCount)
	}
	buf2, ok := m.Memory().Read(offset2, byteCount2)
	if !ok {
		log.Panicf("Memory.Read(%d, %d) out of range (2)", offset2, byteCount2)
	}
	fmt.Printf("%s%s%d\n", buf, buf2, extraInt32)
}

// NewLib creates a Lib. If it succeeds, call Lib.Destroy method
// to release the resouce and Lib.Render method to render a TMD document.
func NewLib() (*Lib, error) {
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)

	// Instantiate a Go-defined module named "env" that exports a function to
	// log to the console.
	_, err := r.NewHostModuleBuilder("env").
		NewFunctionBuilder().WithFunc(printMessages).Export("print").
		Instantiate(ctx)
	if err != nil {
		r.Close(ctx)
		return nil, err
	}

	// Instantiate a WebAssembly module that imports the "log" function defined
	// in "env" and exports "memory" and functions we'll use in this example.
	mod, err := r.InstantiateWithConfig(ctx, tmdWasm,
		wazero.NewModuleConfig().WithStdout(os.Stdout).WithStderr(os.Stderr))
	if err != nil {
		r.Close(ctx)
		return nil, err
	}

	// ...

	var lib = &Lib{
		context: ctx,

		runtime: r,

		module: mod,
		memory: mod.Memory(),

		// inputTmdDataLength: 0, // defaultly
	}

	lib.apiLibVersion = makeWasmApi(lib, "lib_version")
	lib.apiBufferOffset = makeWasmApi(lib, "buffer_offset")
	lib.apiTmdToHtml = makeWasmApi(lib, "tmd_to_html")
	lib.apiTmdFormat = makeWasmApi(lib, "tmd_format")

	return lib, nil
}

// Destroy releases the resource allocated for a Lib.
func (lib *Lib) Destroy() {
	lib.runtime.Close(lib.context)
}

func (lib *Lib) readCString(offset uint32) []byte {
	var end = offset
	if size := lib.memory.Size(); end < size {
		for {
			if c, ok := lib.memory.ReadByte(end); !ok {
				panic("out of range")
			} else if c == 0 {
				break
			}

			end += 1
			if end == size {
				break
			}
		}
	}

	bytes, ok := lib.memory.Read(offset, end-offset)
	if !ok {
		panic("out of range")
	}
	return bytes
}

// Version returns the version of library.
func (lib *Lib) Version() (version []byte, err error) {
	versionOffset, err := lib.apiLibVersion.call()
	if err != nil {
		return nil, err
	}

	return lib.readCString(versionOffset), nil
}

func (lib *Lib) bufferOffset() (uint32, error) {
	offset, err := lib.apiBufferOffset.call()
	if err != nil {
		return 0, err
	}

	return offset, nil
}

func (lib *Lib) writeData(memoryOffset uint32, data []byte) error {
	if !lib.memory.WriteUint32Le(memoryOffset, uint32(len(data))) {
		return fmt.Errorf("Memory.WriteUint32Le(%d, %d) not okay", memoryOffset, len(data))
	}

	if !lib.memory.Write(memoryOffset+4, data) {
		return fmt.Errorf("Memory.WriteString(%d, %s) not okay", memoryOffset+4, data)
	}

	return nil
}

func (lib *Lib) readData(memoryOffset uint32) ([]byte, error) {
	outputLength, ok := lib.memory.ReadUint32Le(memoryOffset)
	if !ok {
		return nil, fmt.Errorf("Memory.ReadUint32Le(%d) not okay (output length)", memoryOffset)
	}
	if outputLength == 0 {
		return nil, nil
	}

	output, ok := lib.memory.Read(memoryOffset+4, outputLength)
	if !ok {
		return nil, fmt.Errorf("Memory.Read(%d, %d) not okay (output length)", memoryOffset+4, outputLength)
	}

	return output, nil
}

// SetInputTmd prepares the input TMD data for later using.
func (lib *Lib) SetInputTmd(tmdData []byte) error {
	bufferOffset, err := lib.bufferOffset()
	if err != nil {
		return err
	}

	err = lib.writeData(bufferOffset, tmdData)
	if err != nil {
		return err
	}

	lib.inputTmdDataLength = uint32(len(tmdData))
	return nil
}

// The options used in HTML generation.
type HtmlGenOptions struct {
	// Enabled custom app name list.
	// Comma or semocolon seperated.
	// Now, only "html" is supported.
	EnabledCustomApps string `json:"enabledCustomApps"`
	// A suffix which will be appended to all HTML id attribute values.
	IdentSuffix string `json:"identSuffix"`
	// A suffix which will be appended to all auto-generated HTML id attribute values.
	AutoIdentSuffix string `json:"autoIdentSuffix"`
	// Whether or not render the root block.
	RenderRoot bool `json:"renderRoot"`
}

const genOptionsTempliate = `
@@@ #enabledCustomApps
'''
%s
'''

@@@ #identSuffix
'''
%s
'''

@@@ #autoIdentSuffix
'''
%s
'''

@@@ #renderRoot
'''
%v
'''
`

func makeGenOptionsData(options HtmlGenOptions) []byte {
	return fmt.Appendf(make([]byte, 0, 2000),
		genOptionsTempliate,
		options.EnabledCustomApps,
		options.IdentSuffix,
		options.AutoIdentSuffix,
		options.RenderRoot,
	)
}

// GenerateHtml converts the current set input TMD into HTML.
func (lib *Lib) GenerateHtml(options HtmlGenOptions) (htmlData []byte, err error) {
	bufferOffset, err := lib.bufferOffset()
	if err != nil {
		return nil, err
	}
	bufferOffset += 4 + lib.inputTmdDataLength

	var optionsData = makeGenOptionsData(options)
	err = lib.writeData(bufferOffset, optionsData)
	if err != nil {
		return nil, err
	}

	outputOffset, err := lib.apiTmdToHtml.call()
	if err != nil {
		return nil, err
	}

	return lib.readData(outputOffset)
}

// GenerateHtmlFromTmd converts a TMD document into HTML.
func (lib *Lib) GenerateHtmlFromTmd(tmdData []byte, options HtmlGenOptions) (htmlData []byte, err error) {
	err = lib.SetInputTmd(tmdData)
	if err != nil {
		return nil, err
	}
	return lib.GenerateHtml(options)
}

// Format formats the current set input TMD.
func (lib *Lib) Format() (formattedData []byte, err error) {
	bufferOffset, err := lib.bufferOffset()
	if err != nil {
		return nil, err
	}
	bufferOffset += 4 + lib.inputTmdDataLength

	err = lib.writeData(bufferOffset, []byte{})
	if err != nil {
		return nil, err
	}

	formatOffset, err := lib.apiTmdFormat.call()
	if err != nil {
		return nil, err
	}

	return lib.readData(formatOffset)
}

// FormatTmd formats a TMD document.
func (lib *Lib) FormatTmd(tmdData []byte) (formattedData []byte, err error) {
	err = lib.SetInputTmd(tmdData)
	if err != nil {
		return nil, err
	}

	return lib.Format()
}
