package azure_functions_def

import (
	"fmt"

	"github.com/weka/go-cloud-lib/functions_def"
)

type AzureFuncDef struct {
	baseFunctionUrl string
	functionKey     string
}

func NewFuncDef(baseFunctionUrl, functionKey string) functions_def.FunctionDef {
	return &AzureFuncDef{
		baseFunctionUrl: baseFunctionUrl,
		functionKey:     functionKey,
	}
}

// each function takes json payload as an argument
// e.g. "{\"hostname\": \"$HOSTNAME\", \"type\": \"$message_type\", \"message\": \"$message\"}"
func (d *AzureFuncDef) GetFunctionCmdDefinition(name functions_def.FunctionName) string {
	functionUrl := d.baseFunctionUrl + string(name)
	var funcDef string
	if name == functions_def.Protect {
		funcDefTemplate := `
		function %s {
			echo "%s function is not implemented"
		}
		`
		funcDef = fmt.Sprintf(funcDefTemplate, name, name)
	} else {
		funcDefTemplate := `
		function %s {
			local json_data=$1
			curl %s?code=%s -H 'Content-Type:application/json' -d "$json_data"
		}
		`
		funcDef = fmt.Sprintf(funcDefTemplate, name, functionUrl, d.functionKey)
	}

	return funcDef
}
