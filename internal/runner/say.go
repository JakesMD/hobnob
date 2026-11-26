package runner

import (
	"fmt"

	"hobnob/internal/config"
	"hobnob/internal/eval"
	"hobnob/internal/scope"
)

// execSay shows a say: step's message and waits for the person to go on.
// With prompts off nobody is there to wait for, so it only logs the message.
func execSay(execState execCtx, step config.Step, scope *scope.Scope) error {
	message, err := eval.EvalTemplate(step.Message, scope.Vars())
	if err != nil {
		return fmt.Errorf("say: %w", err)
	}
	message = scope.Mask(message)
	if !execState.noPrompts {
		resume := execState.log.Pause()
		err := execState.prompts.Say(execState.ctx, message, execState.task)
		resume()
		if err != nil {
			return wrapPromptErr("say", err)
		}
	}
	execState.events.Say(execState.task, message)
	return nil
}
