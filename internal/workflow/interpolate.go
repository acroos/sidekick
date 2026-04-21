package workflow

import "os"

// Interpolate expands $VARIABLE and ${VARIABLE} references in all step
// Run and Prompt fields using the provided variables map.
func Interpolate(wf *Workflow, variables map[string]string) {
	expand := func(key string) string {
		return variables[key]
	}
	for i := range wf.Steps {
		wf.Steps[i].Run = os.Expand(wf.Steps[i].Run, expand)
		wf.Steps[i].Prompt = os.Expand(wf.Steps[i].Prompt, expand)
	}
}
