// Package e2e drives hobnob end-to-end: a hobnob.yml (or a small set of
// files) goes in, app.Run executes it for real, and assertions read what the
// user would actually see — printed output, exit code, prompts asked. See
// docs/ for the language this suite is a spec of.
//
// Mutation checklist — each line is a one-statement break that some e2e test
// in this package must catch. Run through this list after any change to the
// harness itself, and in full before a release; it's a stronger signal than
// a coverage percentage because it tests the assertions, not just execution.
//
//  1. reverse matrix nesting order in internal/runner/loop.go        -> loop_test.go (.Lines)
//  2. drop the raw-value ReplaceAll in Scope.Mask                    -> secrets_test.go (.Masked)
//  3. drop the JSON-escaped ReplaceAll in Scope.Mask                 -> secrets_test.go (into: leaf)
//  4. drop the claimed guard on cliVars in scope.resolve             -> scope_test.go (ConstWinsOverCLIArgs)
//  5. remove the "_" guard in internal/app/app.go Run                -> cli_test.go
//  6. make Task.Hidden not suppress from --list                      -> cli_test.go (.NotOut)
//  7. make module flatten override a native task                    -> modules_test.go
//  8. skip Scope.Copy() before execCall                              -> call_test.go isolation
//  9. evaluate set: entries bottom-to-top                            -> set_test.go
//
// 10. make a later env: entry lose to an earlier one                 -> env_test.go
// 11. resolve relative dir: against cwd instead of TaskfileDir       -> dir_test.go
// 12. make value.Capture always return a String                     -> values_test.go
// 13. drop the Check set on a menu's ask.Question in runner.getInteractive -> get_test.go (InteractiveSelectRepromptsUntilCheckPasses)
// 14. make step-level dir: lose to task-level                        -> dir_test.go
// 15. drop the missing-env-file warning in scope/envfiles.go         -> env_test.go stderr
//
// 16. make asNumber assert Any().(string) instead of String()        -> compare_test.go (YAML int literal)
//
// 17. key the once: memo on task name instead of Steps identity      -> once_test.go (ModuleTaskIdentitySharesMemo...)
// 18. skip storing the child scope in callMemo after a first run     -> once_test.go (MemoizedSecondCallSkipsRerun, ReplaysAcrossCallSandboxes)
// 19. drop the use:/rerun: parse-time rejection in config/steps.go   -> once_test.go (UseRejectedAtParseTime, RerunRejectedAtParseTime)
//
// 20. make run:'s Array-splice loop stringify the whole Array        -> run_test.go (ArrayElementSplices)
// 21. drop the empty-argument-preserved rule in literal.Argv.Resolve -> run_test.go (EmptyElementPreserved)
// 22. allow run: [] to parse instead of erroring                     -> run_test.go (EmptyListIsParseError)
//
// 23. make hbpath return Nil instead of the missing sentinel         -> accessor_test.go (MissingPathErrorsNamingThePath)
// 24. drop the IsMissing trigger in filterDefault                    -> accessor_test.go (DefaultCatchesMissingPath)
// 25. let a wrong-kind access return missing, not an error           -> accessor_test.go (WrongKindNotCaughtByDefault)
// 26. drop the marker scan in eval.EvalTemplate                      -> accessor_test.go (bare {{ .A.b }} render)
// 27. keep no-match elements under [*] instead of dropping them      -> accessor_test.go (WildcardMapsAndDropsNoMatchElements)
// 28. stop skipping string literals in rewriteAccessors              -> accessor_test.go (StringLiteralBracketsSurviveUnchanged)
// 29. drop the head/tail split in eval.EvalIntoLeaf                  -> accessor_test.go (IntoSourceWithAccessorAndFilterChain)
// 30. narrow eval.IsBareRef back to ^\.[A-Z][A-Z0-9_]*$               -> accessor_test.go (BraceFreeOptionsForm, BraceFreeDirForm)
//
// 31. drop the ambient-flag clear in scope.Scope.Set                 -> modules_test.go (SetStepOnOSEnvNameBeatsModuleVarsDefault)
// 32. drop the secret flag from runner.captureInto's Set            -> call_test.go (ChildSecretPulledByAccessorIsMasked)
// 33. return an empty value for an unset name in config.CallSource   -> call_test.go (IntoMissingKeyErrors)
//
// 34. drop signalGroupTerm from stopLeftovers                       -> run_background_test.go (JobGetsSigtermWhenHobnobExitsAndItsOutputIsKept)
// 35. skip the groupHasOthers check in execRun                       -> run_background_test.go (JobNotHoldingTheOutputIsKilledWhenHobnobExits, JobIsKilledWhenHobnobExits)
// 36. hand os/exec the LineWriters instead of attachPipes' files     -> run_background_test.go (StepFinishesWhenShellExits)
// 37. drop killLeftovers from KillRunningStep                        -> run_background_test.go (SecondCtrlCKillsJobsHobnobIsWaitingOn, waits a minute)
// 38. drop the defer stopLeftovers from ExecuteTask                  -> run_background_test.go (JobIsKilledWhenHobnobExits, JobIsKilledWhenALaterStepFails)
//
// 39. check envSoFar before scope.claimed in Scope.envFileRef        -> env_test.go (DotenvReferenceReadsFinalValueOfCLIArg)
// 40. drop the unterminated-quote error in scope.parseDotenv         -> env_test.go (DotenvUnterminatedQuoteFailsNamingFileAndLine)
// 41. close ${ at the first } instead of matchingBrace               -> env_test.go (DotenvDefaultAndUnsetReferences)
// 42. drop the text-after-closing-quote error in parseDotenv         -> env_test.go (DotenvMalformedLineFailsNamingLine)
//
// 43. drop the "_" guard in config.CallSource                        -> call_test.go (IntoCannotReadPrivateChildVar)
// 44. drop the run: bare-entry check in parseStepNode                -> call_test.go (IntoBareNameOnRunRejectedAtParseTime)
// 45. pull results: even when the call: step has its own into:       -> results_test.go (IntoReplacesResults)
// 46. drop the results: presence check in config.CallSource          -> results_test.go (PromiseCheckedEvenWithInto)
// 47. use step.IntoEntries, not CallInto, in stepKinds' call: writes -> results_test.go (BareCallClashingWithConstFailsBeforeAnyStepRuns)
// 48. drop the leading-_ allowance in eval.isUpperHeadTerm           -> accessor_test.go (BraceFreePrivateVarForm)
// 49. test len(IntoEntries) instead of HasInto in config.CallInto    -> results_test.go (EmptyIntoTakesNothing, EmptyIntoLoadsDespiteConstResult)
// 50. drop the DeclaresConst check in runner.finishCall              -> results_test.go (ModuleResultClashingWithCallerConstFails)
//
// 51. append a spread's list whole in literal's node.eval            -> set_test.go (SpreadSplicesListsIntoAListLiteral), call/const/vars/run_into _test.go (ListLiteralSpreads...)
// 52. make literal.intoLeaf never report a spread                    -> run_into_test.go (ListLiteralSpreadsCapturedList, SpreadOutsideAListErrors)
// 53. drop the child.spread check on object fields in literal.parseTree -> set_test.go (SpreadOutsideAListErrors)
// 54. wrap run: list elements with eval.IsBareRef alone in literal.ParseArgv -> run_test.go (SpreadIsANoOp)
// 55. let literal.singleLeaf return a spread's template instead of erroring -> loop_test.go (SpreadTargetErrors), set/run_into _test.go (SpreadOutsideAListErrors)
// 56. let config.stepKindOf keep the last of two kind keys           -> parse_test.go (StepWithRunAndSetRejected, StepWithLoopAndRunRejected)
// 57. drop the foreign-key loop in config.stepKindOf                 -> parse_test.go (KeyForeignToStepKindRejected, UnknownStepKeyRejected)
// 58. return only ITEM for a templated source in loopIterators       -> const_test.go (ReservedNameRejectsTemplatedLoopKeyAndValue)
// 59. skip the loop: body in Step.Writes                             -> const_test.go (ReservedNameCheckedInsideNestedLoopSteps)
// 60. call scope.checkLoadRules from Load for the root only, not resolve -> modules_test.go (OwnConstReadingImporterConstFailsAtLoad, OwnVarsSelfReferenceFailsAtLoad, OwnEnvPathReferencingOwnVarsNameFailsAtLoad)
// 61. drop the with: keys from stepKinds' call: writes             -> const_test.go (ReservedNameRejectsWithCollision, WithCollisionCheckedInsideLoopSteps)
// 62. write cliVars with value.Str, not value.Capture, in scope.resolve -> values_test.go (CLIArgThatIsJSONBecomesStructure)
// 63. return a String multi: preset as is in runner.multiList        -> get_test.go (MultiPresetNotAJSONListFails, MultiPresetEmptyTextIsEmptyList)
// 64. drop the JSON-text case from runner.multiList                  -> get_test.go (MultiPresetFromShellJSONTextBecomesAList, MultiDefaultAsJSONTextBecomesAList)
// 65. signal the group as soon as the shell exits, in execRun         -> run_background_test.go (JobKeepsRunningAfterItsStepEnds, JobNotHoldingTheOutputKeepsRunningAfterItsStepEnds)
// 66. keep a quiet: step's LineWriters as its late dest in execRun    -> run_background_test.go (QuietStepsJobStaysQuiet)
// 67. drop the absolute-path rewrite for an already-set type: file value -> get_path_test.go (PresetRelativeBecomesAbsolute)
// 68. resolve a CLI-arg path against the task dir, not the invocation dir -> get_path_test.go (CLIArgResolvesAgainstInvocationDir, ShellEnvResolvesAgainstInvocationDir)
// 69. skip the right-kind check in ask.PathRules.Verify -> get_path_test.go (FilePresetThatIsADirectoryFails, DirPresetThatIsAFileFails), ask path_test.go (PathQuestion_JudgesEachPickedPath)
// 70. resolve symlinks before storing a picked path                   -> get_path_test.go (SymlinkKeptAsGiven)
// 71. treat a directory default on type: file as the answer with prompts off -> get_path_test.go (DirectoryDefaultIsNoAnswerWithPromptsOff, DirectoryDefaultOptionalIsEmptyWithPromptsOff, MultiDirectoryDefaultIsNoAnswerWithPromptsOff, MultiDirectoryDefaultOptionalIsEmptyWithPromptsOff)
// 72. set optional entries empty before reading default: in getFromNoPrompts -> get_test.go (OptionalWithDefaultUsesDefaultWithPromptsOff)
// 73. check a multi: list's match: on the joined text, not per item in runner.acceptPath -> get_path_test.go (EachMultiItemMustMatch)
// 74. let a plain match: pattern hide directories in the file picker (ask.PathRules.List) -> ask path_test.go (PathRules_ListShowsWhatCanBeBrowsed)
// 75. drop the options:+type: load-time rejection                     -> get_path_test.go (OptionsWithTypeFileRejectedAtLoad)
// 76. drop the Check set on a picker's ask.Question in runner.getPathInteractive -> get_path_test.go (FailingCheckRefusesThePick)
// 77. leave the Path rules off the picker's ask.Question in runner.getPathInteractive -> get_path_test.go (PickerAnswerIsStoredAbsolute)
// 78. drop the Included/Excluded assignments in runner.pathRules -> get_path_test.go (PickerGetsMatchAsPredicates)
// 79. print a secret value in full in runner.logVar -> log_lines_test.go (VariableLinesNameTheValueAndWhereItCameFrom, secret cases)
// 80. skip the allowed test on multi: defaults in ask.PathRules.Opening -> ask path_test.go (PathRules_OpeningStartsAtTheDefaultAndPicksWhatFits)
// 81. list a colon-named task ungrouped, or a plain task under a heading, in cli.groupTaskRows -> cli_test.go (ListGroupsTasksByNamePrefix)
// 82. let esc skip a required text prompt in tui.textModel.Update    -> prompt_text_test.go (Update_Answers)
// 83. drop the error line in tui.card.render                         -> prompt_text_test.go (Update_Answers)
// 84. print no call line in runner.execCall                          -> log_lines_test.go (CallPrintsTheCallLine, CallToASkippedTaskPrintsTheCallThenTheSkip)
// 85. skip DoneEarlier on a once: cache hit                          -> log_lines_test.go (CallToAnAlreadyRunOnceTaskIsDoneEarlier)
// 86. drop the continuation indent in tui.RunLog.StepStarted       -> log_lines_test.go (MultiLineCommandContinuesUnderTheFirstLine)
// 87. leave a quiet: step's note off its run line in runner.execRun  -> log_lines_test.go (QuietStepNotesHiddenOutput)
// 88. print a skipped run: step as "skipped" instead of its command  -> run_into_test.go (IfFalseSkipsAndPrintsSkipLine)
// 88a. print an unrenderable skipped run: step as "skipped" in runner.skippedRunText -> run_into_test.go (SkippedStepWhoseCommandCannotRenderPrintsItAsWritten)
// 89. leave the source note off a variable line in runner.logVar -> log_lines_test.go (VariableLinesNameTheValueAndWhereItCameFrom)
// 90. print a skipped optional get: as an empty value, not ⊘ NAME -> log_lines_test.go (SkippedOptionalPromptPrintsAMarkedName, SkippedOptionalWithPromptsOffPrintsAMarkedName)
// 90a. log any empty get: value as ⊘ NAME, not only an optional one's in runner.logVar -> log_lines_test.go (EmptyValueForARequiredVariablePrintsAVariableLine)
// 91. let enter pick a task when the picker's filter matches nothing, in tui.taskSelectModel.Update -> prompt_taskselect_test.go (EnterWithNothingMatchingStaysOpen)
// 92. skip the step end line in runner.endRunStep                    -> log_ends_test.go (StepAndTaskEndWithTheirOwnLines)
// 93. skip the task end line for a called task in runner.executeTask -> log_ends_test.go (FailureTwoCallsDeepEndsEveryTaskInnermostFirst)
// 94. log a soft failure with StepFailed in runner.endRunStep        -> log_ends_test.go (SoftFailureEndsWithABangAndTheTaskCountsIt)
// 95. drop the soft count increment in runner.executeSteps           -> log_ends_test.go (SoftFailureEndsWithABangAndTheTaskCountsIt)
// 96. drop the StepError for an unreported error in runner.executeSteps -> log_ends_test.go (StepErrorWithoutARunEndsTheStepWithTheMessage)
// 97. let enter open a directory in a file picker (tui.pathModel.Update) -> prompt_path_test.go (EnterOnADirectoryInAFilePicker_ShowsThatRightOpensIt)
// 98. let backspace delete the path line's leading "/" (tui.pathModel.Update) -> prompt_path_test.go (BackspaceCannotDeleteTheLeadingSlash)
// 99. drop the Stopping call in runner.executeTask                    -> log_ends_test.go (InterruptedRunSaysStoppingOnceThenEndsEveryTaskStopped)
// 100. print a stopped task's end as "failed" in runner.executeTask   -> log_ends_test.go (InterruptedRunSaysStoppingOnceThenEndsEveryTaskStopped)
// 101. drop the step as written from the message in runner.stepMessage -> log_errors_test.go (StepThatCannotStartIsOneLineWithTheStepAsWritten)
// 102. skip eval.plainTemplateError on a template's Execute error     -> log_errors_test.go (StepThatCannotStartIsOneLineWithTheStepAsWritten)
// 103. report a missing dir as the command's fork/exec error in runner.startFailure -> log_errors_test.go (CommandThatCannotStartEndsLikeAnyFailedStep)
// 104. print an App.Run error without the hobnob ✗ line               -> log_errors_test.go (BeforeARunHobnobTakesTheTasksPlace)
// 105. print the missing env file warning with the old warning: word in scope.loadEnvFiles -> log_errors_test.go (MissingEnvFileWarnsAsHobnobAndTheRunCarriesOn)
// 106. print the demo notice without hobnob » in app.announceDemo     -> log_errors_test.go (DemoNoticeIsAHobnobNote)
// 107. write the installer's output without the hobnob │ prefix in app.runInstaller -> internal/app/upgrade_test.go (TestUpgradeWritesInTheLogsStyle)
// 108. drop `shellCmd.Stdin = os.Stdin` in runner.execRun             -> run_pty_test.go (CommandInheritsStdin)
// 109. pass no ptySize to attachPipes in runner.execRun              -> run_pty_test.go (WatchedCommandSeesATTYOnStdoutOnly)
// 110. drop the prefix width from tui.RunLog.PTYSize                 -> run_pty_test.go (WidthIsTheTerminalsLessThePrefix)
// 111. drop the pipes.resize call from the OnResize callback         -> run_pty_test.go (ResizeReachesTheRunningCommand)
// 112. never wrap in tui.LineWriter.text                             -> run_pty_test.go (LongLinesWrapUnderTheirTextOnlyOnATerminal), linewriter_test.go
// 113. drop the cursorMove check in tui.LineWriter.Write             -> run_pty_test.go (CursorMovingOutputHasNoPrefix), linewriter_test.go
// 114. buffer text until a newline when watched, in LineWriter.text  -> linewriter_test.go (Watched_Write, a prompt with no newline)
// 115. skip the "!" line for a failed soft call: in runner.executeSteps -> log_ends_test.go (SoftCallThatFailsEndsTheCallingStepWithABang)
// 116. print nothing for a skipped call: step in runner.executeSteps -> log_lines_test.go (SkippedCallNamesTheCalledTask)
// 117. leave the "# default" note off a --no-input default in runner.getFromNoPrompts -> log_lines_test.go (DefaultTakenWithPromptsOffIsNoted)
// 118. word the no-taskfile error "found in … any parent directory" in the taskfile search of app/args.go -> log_errors_test.go (BeforeARunHobnobTakesTheTasksPlace), demo_test.go (NotReachableWithoutTheFlag)
// 119. leave a run: command's notes on its line when quiet, in tui.RunLog.StepStarted -> log_quiet_test.go (QuietRunKeepsOnlyCommandLinesAndTaskEnds)
// 120. drop the HOBNOB_QUIET=1 check in app.run                       -> log_quiet_test.go (QuietRunKeepsOnlyCommandLinesAndTaskEnds, QuietIsNeverOnByItself)
// 121. stop holding a step's output when the log is quiet, in runner.execRun -> log_quiet_test.go (QuietRunKeepsOnlyCommandLinesAndTaskEnds, QuietRunPrintsAFailedStepsHeldOutputBeforeItsEnd)
//
// 122. forward a CTRL+C byte to the PTY instead of signalling hobnob    -> runner/run_stdin_test.go (CtrlCReachesHobnobNotTheCommand)
// 123. drop the terminal restore in runner.attachStdin's stop           -> runner/run_stdin_test.go (ForwardsKeysToTheCommand)
//
// 124. drop the "check failed:" prefix from a check's error in ask.Question.checked -> prompt_select_test.go (ErroringCheckShowsItsError)
// 125. let enter with nothing toggled on an optional multi menu go through the check (tui.selectModel.confirm) -> prompt_select_test.go (OptionalMultiEnterWithNothingToggledSkips)
// 126. drop the check-error refusal in ask.Question.checked -> get_path_test.go (ErroringCheckRefusesThePickShowingItsError), prompt_path_test.go (RefusalFromTheJudgeShowsOnTheErrorLineAndKeepsThePickerOpen)
// 127. stop dropping the win32-input and focus-reporting mode switches in runner.conPTYFilter.Write -> internal/runner/conptyfilter_test.go (DropsTerminalModeSequencesSplitAcrossWrites)
// 128. write a held unfinished escape nowhere in runner.conPTYFilter.Flush -> internal/runner/conptyfilter_test.go (PassesOnAnUnfinishedEscape)
// 129. (windows) create the process without the stderr pipe handle in runner.startOnPTY -> run_conpty_windows_test.go (WatchedCommandSeesATTYOnStdoutOnly)
// 130. write the rule to a piped stdout (create the live area without a Screen in tui.RunLog.Watch) -> log_status_test.go (NeverAppearsInPipedOutput), live_test.go (NeverReachesAPipe)
// 131. drop the live.begin call from tui.RunLog.TaskEntered        -> log_status_test.go (RuleShowsUnderTheRunningCommandAndIsGoneAfter), live_test.go
// 132. drop the live.end call from tui.RunLog.TaskLeft -> live_test.go (IsGoneWhenTheRunEnds), log_status_test.go
// 133. drop the step clock (live.stepBegin in tui.RunLog.StepStarted)         -> live_test.go (SitsUnderTheLogWithTheStepAndTotalClocks, BetweenStepsHasOnlyTheTotal)
// 134. drop the note from tui.RunLog.Stopping                         -> live_test.go (SaysStoppingOnItsLeftAfterCtrlC)
// 135. drop the clear in tui.liveArea.pause                          -> live_test.go (IsClearedWhileACardPromptIsOpen)
// 136. skip the cursorMove check in tui.liveArea.write                -> live_test.go (HidesWhileAToolRedrawsRowsAndReturnsWhenItStops)
// 137. never notice a waiting prompt in tui.liveArea.redrawLocked     -> live_test.go (StaysUnderAPromptWaitingForInputWithItsClockFrozen)
// 138. drop the partial-line write in runner.promptWriter.Write       -> log_quiet_test.go (QuietRunStillShowsAPromptButNotWholeLines)
// 139. drop the field as written from execSet, execFor or execGet errors -> log_errors_test.go (StepThatCannotStartIsOneLineWithTheStepAsWritten)
// 140. print Go's raw JSON syntax error from value.filterJSON            -> log_errors_test.go (StepThatCannotStartIsOneLineWithTheStepAsWritten)
// 141. write a piped partial line only at its newline in tui.LineWriter.text -> log_quiet_test.go (PipedPartialLineContinuesAsOnePrefixedLine), linewriter_test.go
// 142. drop the prefix of lines before a cursor move in tui.LineWriter.Write -> live_test.go (HidesWhileAToolRedrawsRowsAndReturnsWhenItStops)
// 143. never unhide in tui.liveArea.write after drawIdle                   -> live_test.go (HidesWhileAToolRedrawsRowsAndReturnsWhenItStops)
// 144. write once per grapheme in tui.LineWriter.text                       -> linewriter_test.go (ReachesTheTerminalInOneWritePerChunk)
// 145. take the owed prefix from the area, not the writing step, in tui.liveArea.write -> live_test.go (LateWriteFromAnEarlierStepUsesItsOwnPrefix)
// 146. drop tui.ClearStatusLine from the forced exit in cmd/hobnob/main.go -> live_test.go (ClearStatusLineLeavesNoRuleBehind)
// 147. discard the stdin PTY's output in runner.attachStdin instead of copying it to the terminal -> runner/run_stdin_test.go (ShowsWhatTheCommandWritesToItsTerminalWhenStdoutIsPiped)
// 148. make tui.LineWriter.Restart write col directly instead of flagging -> live_test.go (RestartingTheOtherStreamsWriterIsRaceFree, under -race)
// 149. drop the leading blank line in tui.card.render -> frame_test.go (OpensWithOneBlankLine)
// 150. stop skipping the opening sequences in runner.conPTYFilter.Write -> runner/conptyfilter_test.go (OpeningSequencesLeaveTheOutputPrefixedOnBothStreams)
// 151. drop the required-answer refusal in ask.Question.judgeText -> runner/get_test.go (TextJudge_AppliesDefaultRequiredAndSkipRules), tui/prompt_text_test.go (Update_Answers, required prompt on an empty field)
// 152. drop the empty-picks refusal in tui.selectModel.confirm -> tui/prompt_select_test.go (MultiEnter_NoSelections)
//
// 153. drop the token compare in browser.Server.guard                       -> browser_test.go (RequestsNeedTheToken)
// 154. bind browser.Listen to all interfaces instead of 127.0.0.1           -> browser_test.go (TaskWithoutPromptsEndsOnDone)
// 155. show a held step's last line in browser.state.snapshot (drop the held check) -> browser_test.go (HeldStepShowsNoLastLineAndASecretIsMasked)
// 156. pass an identity mask instead of scope.Mask to runevents.Stream.Tap in runner.execRun -> browser_test.go (HeldStepShowsNoLastLineAndASecretIsMasked)
// 157. drop the OutputLine report from runevents.Stream.Tap -> runevents/stream_test.go (TapPassesOutputOnAndReportsEachMaskedLine), browser_test.go (QuietChangesOnlyTheTerminal)
// 158. record the outermost running task instead of the step's task in runevents.Stream.recordFailure -> runevents/stream_test.go (FinishNamesTheStepThatFailed), runner/outcome_test.go (RunOutcomeNamesTheCalledTaskWhoseStepFailed), browser_test.go (FailedNamesTheStepAndExitsLikeTheTerminalRun)
// 159. never mark a task failed in runevents.Stream.TaskFailed -> runevents/stream_test.go (TaskLeftSaysWhetherItsTaskFailed), browser_test.go (FailedNamesTheStepAndExitsLikeTheTerminalRun)
// 160. label a task by its name even when it has an info: in browser.label  -> browser_test.go (ProgressShowsTheLabelTheLastLineAndTheRail)
// 161. drop the server.AwaitDelivery call from browserFrontEnd.Done (named task)  -> browser_test.go (PageOpenedAfterTheRunEndedStillGetsTheOutcome)
// 162. accept --browser beside --no-input in app.checkBrowserFlag               -> browser_test.go (FlagsThatCannotCombine)
// 163. call app.checkBrowserFlag before --quiet is stripped, in app.run       -> browser_test.go (FlagsThatCannotCombine, quiet cases)
// 164. drop the runner.CheckTask call from browserFrontEnd.Begin               -> browser_test.go (UnknownTaskFailsLikeTheTerminalRunWithoutOpeningAPage)
// 165. drop the forgiven-failure reset from runevents.Stream.StepSoftFailed -> runevents/stream_test.go (FinishIgnoresAFailureASoftStepForgave), runner/outcome_test.go (RunOutcomeIgnoresASoftFailedStep), browser_test.go (FailedNamesTheFailureThatEndedTheRunNotAForgivenOne)
// 166. drop the Flush calls after the copiers finish in runner.execRun    -> browser_test.go (OutputWithoutATrailingNewlineIsTheLastLineAndInTheLog)
// 167. keep ANSI sequences in the line tui.TapWriter.Flush reports -> browser_test.go (LastLineHasNoColourCodes)
// 168. leave elapsed time out of browser.state.snapshot -> browser_test.go (ProgressSnapshotsCarryElapsedTime)
// 169. never mark a rail entry done in browser.state.TaskLeft -> browser_test.go (ProgressShowsTheLabelTheLastLineAndTheRail)
// 170. drop the r.Host check in browser.Server.guard                         -> browser_test.go (RequestsNeedTheLoopbackHostAndAMatchingOrigin)
// 171. drop the Origin check in browser.Server.guard                          -> browser_test.go (RequestsNeedTheLoopbackHostAndAMatchingOrigin)
// 172. accept --browser on Windows in app.checkBrowserFlag                        -> internal/app/args_test.go (BrowserFlagFailsOnWindows)
// 173. skip the Judge call in browser.state.PromptAsk                       -> browser_prompt_test.go (RefusedAnswerKeepsThePromptWithTheReason)
// 174. let browser.state.skip skip a required prompt                         -> browser_prompt_test.go (SkipContinuesAnOptionalPromptWithoutAnAnswer)
// 175. send a secret prompt's default to the page in browser.state.PromptAsk -> browser_prompt_test.go (SecretPromptIsHiddenAndMaskedOnTheRailAndInTheLog)
// 176. put a secret answer on the rail unmasked in browser.state.PromptAsk    -> browser_prompt_test.go (SecretPromptIsHiddenAndMaskedOnTheRailAndInTheLog)
// 177. drop the prompt id check in browser.state.openPrompt                   -> browser_prompt_test.go (AnswerToAPromptThatIsNotOpenIsRefused)
// 178. pass runner.TerminalPrompts() instead of browserPrompts in browserFrontEnd.Begin -> browser_prompt_test.go (TextPromptShowsInfoAndDefaultAndTakesTheAnswer)
// 179. drop the stop call for the first press in browser.Server.Cancel -> browser_test.go (CancelStopsAStepGracefullyAndTheResultIsCancelled)
// 180. drop the kill call for a later press in browser.Server.Cancel -> browser_test.go (SecondCancelForceStopsAStepThatIgnoresTheFirst)
// 181. never set cancelling in browser.Server.Cancel -> browser_test.go (SecondCancelForceStopsAStepThatIgnoresTheFirst)
// 182. run a default task in app.run when --browser names no task          -> browser_test.go (WithoutATaskThePickerListsTheVisibleTasksEvenWithADefault)
// 183. keep the last run's setup entries in browser.state.startPick           -> browser_test.go (ChoosingATaskRunsItAndHomeLeadsToAFreshSecondRun)
// 184. keep the last run's log in browser.state.startPick                     -> browser_test.go (ChoosingATaskRunsItAndHomeLeadsToAFreshSecondRun)
// 185. offer Home on every finished run in browser.state.snapshot             -> browser_test.go (HomeIsOnlyOfferedWhenHobnobHadNoTask)
// 186. accept any task name in browser.state.choose                           -> browser_test.go (PickerRefusesATaskItDidNotOffer)
// 187. drop the isOpen re-check once browser.state.answer holds the prompt's busy lock -> browser_prompt_test.go (SecondAnswerForOnePromptIsRefusedWithoutRunningItsCheck)
// 188. treat an untouched secret default as a skip in ask.Question.judgeText -> browser_prompt_test.go (ContinueOnAnUntouchedSecretDefaultUsesItAndSkipGivesNone)
// 189. drop the required-answer refusal in ask.Question.judgeText               -> browser_prompt_test.go (EmptyAnswerToARequiredPromptIsRefused)
// 190. drop the check: judgement from ask.Question.checked                      -> browser_menu_test.go (MenuChoiceThatFailsItsCheckIsRefusedWithTheReason)
// 191. take every option a multi menu is answered with, offered or not, in ask.Question.judgeMenu -> browser_menu_test.go (MenuRefusesAnOptionItDoesNotOfferAndAnEmptyMultiChoice)
// 192. drop the multi menu's ticked defaults from browser.state.PromptAsk -> browser_menu_test.go (MultiMenuHasItsDefaultTickedAndTakesSeveralChoices)
// 193. let a required multi menu go through with nothing ticked in ask.Question.judgeMenu -> browser_menu_test.go (MenuRefusesAnOptionItDoesNotOfferAndAnEmptyMultiChoice)
// 194. show what match: excludes in ask.PathRules.List (drop the Excluded check) -> browser_picker_test.go (PickerStartsAtTheDefaultFolderAndHidesWhatMatchExcludes)
// 195. list files that match: doesn't include in ask.PathRules.List -> browser_picker_test.go (PickerStartsAtTheDefaultFolderAndHidesWhatMatchExcludes)
// 196. open a file's folder instead of the default folder in ask.PathRules.Opening -> browser_picker_test.go (PickerStartsAtTheDefaultFolderAndHidesWhatMatchExcludes, FolderPickerStartsWithItsDefaultSelectedAndAnswersAtOnce)
// 197. leave a folder picker's default unselected in ask.PathRules.Opening -> browser_picker_test.go (FolderPickerStartsWithItsDefaultSelectedAndAnswersAtOnce)
// 198. open a path that is not a folder in browser.state.askPicker -> browser_picker_test.go (OpeningAFolderByPostChangesTheListing)
// 199. accept a path match: excludes in ask.PathRules.Verify -> browser_picker_test.go (MultiPickerAnswersWithSeveralPathsAndRefusesBadOnes)
// 200. let a required picker go through with no pick in ask.Question.judgePath -> browser_picker_test.go (OptionalPickerCanBeSkippedAndARequiredOneRefusesAnEmptyAnswer)
// 201. never close the previous tab's replaced channel in browser.Server.serveEvents -> browser_lifecycle_test.go (NewestTabWinsAndTheOldOneIsToldAndClosed)
// 202. keep the timer running while steps run in browser.Server.waitingOnPerson -> browser_lifecycle_test.go (NothingExitsWhileStepsRunAndTheTimerStartsWhenTheRunEnds)
// 203. ignore open tabs in browser.Server.watchGrace -> browser_lifecycle_test.go (AReconnectingTabStopsTheTimer)
// 204. skip the Cancel at a prompt in browser.Server.giveUp -> browser_lifecycle_test.go (NoTabAtAPromptCancelsTheRunAfterTheGraceAsInterrupted)
// 205. drop the abandoned case from browser.Server.AwaitDelivery -> browser_lifecycle_test.go (NoTabAtDoneExitsAfterTheGrace)
// 206. drop the abandoned case from browser.Server.AwaitHome -> browser_lifecycle_test.go (NoTabAtDoneInTheHomeFlowExitsAfterTheGraceWithTheRunsOutcome)
// 207. drop the watchGrace call when a tab closes in browser.Server.serveEvents -> browser_lifecycle_test.go (ClosingTheLastTabAtAPromptStartsTheTimer)
// 208. drop the abandoned case from browser.Server.PickTask -> browser_lifecycle_test.go (NoTabAtTheTaskPickerExitsAfterTheGrace)
// 209. let ask.PathRules.Verify accept a junk-named path -> browser_picker_test.go (JunkNamedPathPostedToAFilePickerIsRefused), ask path_test.go (PathQuestion_JudgesEachPickedPath)
// 210. pre-select the nearest existing folder when a folder picker's default is missing in ask.PathRules.Opening -> browser_picker_test.go (FolderPickerWhoseDefaultDoesNotExistStartsWithNothingSelected)
// 211. let browser.Server.Cancel act without a live run (drop the stop != nil check) -> browser_test.go (CancelAtTheTaskPickerChangesNothingAndTheNextRunStopsGracefully)
// 212. leave the task blank when a run fails outside a step in runevents.Stream.innermost -> runevents/stream_test.go (FinishOutsideAStepNamesTheInnermostTaskStillRunning, FinishFallsBackToTheRunsTaskOnceEverythingLeftCleanly), browser_test.go (FailedNamesTheTaskWhenTheRunFailsOutsideAStep)
// 213. pass task.Info to events.Enter unfilled in runner.executeTask -> browser_test.go (LabelFillsInTheVariablesItCan)
// 214. skip the prompt in runner.execSay -> say_test.go (ShowsTheFilledInMessageAndLogsIt), browser_prompt_test.go (SayShowsTheMessageAndContinueGoesOn)
// 215. drop scope.Mask in runner.execSay -> say_test.go (ShowsTheFilledInMessageAndLogsIt)
// 216. prompt even when prompts are off in runner.execSay -> say_test.go (WithoutPromptsOnlyLogsTheMessage)
// 217. return early under quiet in tui.RunLog.Say -> say_test.go (ShowsUnderQuiet)
// 218. accept an empty or non-scalar say: in config.parseStepNode -> say_test.go (RejectsWhatIsNotAMessage)
// 219. add the say answer to the rail in browser.state.PromptSay -> browser_prompt_test.go (SayShowsTheMessageAndContinueGoesOn)
// 220. listen on port 0 whatever the port in browser.Listen -> browser_test.go (PortServesThePageWithoutATokenAndOpensNoBrowser)
// 221. drop the --port needs --browser check in app.frontEnd -> browser_test.go (PortRefusals)
// 222. accept a port outside 1 to 65535 in app.extractPortFlag -> browser_test.go (PortRefusals)
// 223. open the browser under --port in app.openPage -> browser_test.go (PortServesThePageWithoutATokenAndOpensNoBrowser)
// 224. check the token on a fixed server in browser.Server.guard -> browser_test.go (PortServesThePageWithoutATokenAndOpensNoBrowser)
// 225. keep the token in a fixed server's browser.Server.URL -> browser_test.go (PortServesThePageWithoutATokenAndOpensNoBrowser)
// 226. start the grace timer on a fixed server in browser.Server.watchGrace -> browser_test.go (PortWaitsForATabPastTheGrace)
// 227. run a named task under --port in browserFrontEnd.Begin -> browser_test.go (PortRefusals)
// 228. read a multi menu's JSON-list default as one option's text in runner.menuPicks -> runner/get_test.go (MenuQuestion_ResolvesDefaultsIntoPicks)
// 229. skip the default fallback for an empty text answer in ask.Question.judgeText -> runner/get_test.go (TextJudge_AppliesDefaultRequiredAndSkipRules)
// 230. leave match: out of the text Check in runner.checkCandidate -> runner/get_test.go (TextJudge_RefusesWithMatchAndCheckWording)
// 231. take menu picks in the order given, not the options', in ask.Question.judgeMenu -> runner/get_test.go (MenuJudge_AppliesPickRules)
// 232. emit Stopping on every call, not just the first, in runevents.Stream.Stopping -> runevents/stream_test.go (StoppingReportsOnlyTheFirstInterruptOnTheInnermostTask), live_test.go
// 233. drop the Call line from browser.state's log (leave Call to runevents.Nop) -> browser/state_test.go (DetailsLogCarriesEveryEventTheTerminalShows)
// 234. log the failure line twice, from StepFailed and again from Finished, in browser.state.Finished (drop the failureLogged check) -> browser/state_test.go (FailedResultIsLabelledWithTheNamedTaskAndLogsTheFailureOnce)
// 235. classify an interrupt as Failed in runner.ExecuteTask's Finish call -> browser_test.go (CancelStopsAStepGracefullyAndTheResultIsCancelled)
// 236. skip Finish when ExecuteTask fails before entering a task (resolveTask error) -> runner/outcome_test.go (RunOutcomeForAnUnknownTaskCarriesOnlyTheReason)
// 237. let a named task run under --port in browserFrontEnd.Begin (drop the picked check) -> app/frontend_test.go (BrowserFrontEndRefusesANamedTaskUnderPort)
// 238. drop the AlwaysPicks check so a default task short-circuits --browser -> app/frontend_test.go (BrowserFrontEndPicksFirstTerminalDoesNot), browser_test.go (WithoutATaskThePickerListsTheVisibleTasksEvenWithADefault)
// 239. skip the front end's Done wait after a run in app.runTask -> app/frontend_test.go (SelectAndRunPicksRunsThenWaits)
// 240. stop picking after the first run in app.selectAndRun (ignore Done's again) -> app/frontend_test.go (SelectAndRunPicksAgainWhileTheWaitSaysSo)
// 241. return nil instead of the last run's failure when the picker ends in app.selectAndRun -> app/frontend_test.go (SelectAndRunReturnsTheLastRunsFailureWhenThePickerEnds)
// 242. run a task after Begin refuses it in app.runTask -> app/frontend_test.go (ExecTaskFailsSilentlyWhenTheFrontEndRefusesTheTask)
// 243. accept --port without --browser in app.frontEnd -> app/frontend_test.go (PortNeedsBrowser)
// 244. pick from a front end that cannot pick in app.selectAndRun (skip CanPick) -> app/frontend_test.go (SelectAndRunListsWhenTheFrontEndCannotPick)
// 245. leave the junk-name check out of ask.PathRules.Verify -> ask path_test.go (PathQuestion_JudgesEachPickedPath)
// 246. drop the ~ expansion in ask.PathRules.Resolve -> ask path_test.go (PathQuestion_JudgesEachPickedPath), get_path_test.go (PathIsCleanedAndHomeExpanded)
// 247. list a path's picks more than once in ask.Question.judgePath -> ask path_test.go (PathQuestion_ShapesTheAnswerLikeAMenu)
// 248. accept the answer despite a refusal in tui.pathModel.judge -> prompt_path_test.go (RefusalFromTheJudgeShowsOnTheErrorLineAndKeepsThePickerOpen)
package e2e

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"hobnob/internal/app"
	"hobnob/internal/ask"
	"hobnob/internal/runner"
	"hobnob/internal/termsim"
	"hobnob/internal/tui"
	"hobnob/internal/value"
)

// envMu serializes every e2e test's use of process-global state: os.Environ,
// os.Stdout/Stderr, and the working directory. Tests in this package must
// never call t.Parallel() — see the note on Run below.
var envMu sync.Mutex

// Files maps a relative path to file content. "hobnob.yml" is the entry
// point app.Run is pointed at.
type Files map[string]string

// Case describes one end-to-end scenario.
type Case struct {
	Yml      string              // shorthand for Files{"hobnob.yml": Yml}
	Files    Files               // multi-file, inline; wins over Yml if both set
	Dir      string              // OR: copy this internal/e2e/testdata subdirectory in
	Args     []string            // as the user types, without --file
	Answers  map[string][]string // fake prompt answers by var name; consumed in order, re-tried against check:
	TaskPick string              // answer for the interactive task-select picker
	Env      map[string]string   // the ONLY env vars visible to the run (plus PATH/HOME/TMPDIR)
	TTY      bool                // App.IsTerminal (default false). Set true to reach get:'s interactive
	// path at all — like the real CLI, a non-terminal stdin skips straight to
	// getFromNoPrompts regardless of Answers, so any test asserting on a
	// prompt (Answers, TaskPick, .Prompted) needs TTY: true.
	Interrupt string      // var name whose text prompt ends as if CTRL+C had torn it down
	Probe     []string    // paths (relative to the temp root) a picker's Included/Excluded predicates are asked about; the answers land in PromptCall.Included/Excluded
	Cwd       string      // subdir of the temp root to chdir into (default: the root itself)
	Discover  bool        // omit --file, exercise findTaskfile's upward walk
	Screen    *tui.Screen // the terminal the run is watched on (default nil: stdout is piped)
	Stdin     string      // what hobnob's stdin holds, which a run: command inherits (default: nothing)

	// WhileRunning, with a Screen, runs alongside the run and gets a func
	// returning the rows of the terminal as they are right now. It must end
	// by letting the run finish (a command waiting for a file, say).
	WhileRunning func(rows func() []string)

	// Browser plays the page when a --browser run opens it. It runs alongside the
	// run; a failed check in it ends it, which cancels the run.
	Browser func(b *Browser)
}

// screenOf is a Screen of fixed size, resized by calling the returned func
// with the new size.
func screenOf(cols, rows int) (screen *tui.Screen, resize func(cols, rows int)) {
	var mu sync.Mutex
	var watchers []func()
	screen = &tui.Screen{
		Size: func() (int, int) {
			mu.Lock()
			defer mu.Unlock()
			return cols, rows
		},
		OnResize: func(fn func()) func() {
			mu.Lock()
			defer mu.Unlock()
			watchers = append(watchers, fn)
			return func() {}
		},
	}
	resize = func(c, r int) {
		mu.Lock()
		cols, rows = c, r
		fns := append([]func(){}, watchers...)
		mu.Unlock()
		for _, fn := range fns {
			fn()
		}
	}
	return screen, resize
}

// PromptCall records one prompt the run actually made.
type PromptCall struct {
	VarName  string
	Info     string
	Default  string
	Items    []string
	Multi    bool
	Secret   bool
	Optional bool
	Task     string
	Type     string   // "file" or "dir" for a picker, "say" for a say: step (Info is its message), empty for a text prompt or menu
	Refusals []string // what the judge refused of the answers offered before the one it took

	// What the picker's predicates reported for each Case.Probe path (keyed as
	// given). Nil when the picker got no predicate.
	Included map[string]bool
	Excluded map[string]bool
}

// Result is what the user would have seen.
type Result struct {
	Stdout, Stderr, Combined string

	// Raw is what was written to stdout, escape codes and all. With a Screen,
	// Stdout is what the terminal shows instead: the rows after every erase
	// and redraw of the status line.
	Raw      string
	ExitCode int
	Prompts  []PromptCall
	Dir      string
	URL      string // what --browser opened in the browser, empty if it didn't

	// runErr is unexported so it can't be compared directly (app.ErrSilent's
	// empty message makes that a footgun) — go through .OK/.Fails/.Err.
	runErr error
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// dedent strips the common leading whitespace from a raw-string YAML
// fixture, and its leading/trailing blank lines, so fixtures can be indented
// to match the surrounding Go code.
func dedent(s string) string {
	lines := strings.Split(s, "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	minIndent := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " \t"))
		if minIndent == -1 || indent < minIndent {
			minIndent = indent
		}
	}
	if minIndent <= 0 {
		return strings.Join(lines, "\n") + "\n"
	}
	out := make([]string, len(lines))
	for i, line := range lines {
		if len(line) >= minIndent {
			out[i] = line[minIndent:]
		} else {
			out[i] = strings.TrimLeft(line, " \t")
		}
	}
	return strings.Join(out, "\n") + "\n"
}

// Yml is the 80% shorthand: a single-file fixture and the args to run it
// with.
func Yml(t *testing.T, yml string, args ...string) *Result {
	t.Helper()
	return Run(t, Case{Yml: yml, Args: args})
}

// Run writes c's files into a fresh temp dir, points a real app.Run at them
// with every seam (terminal, prompts, task picker, environment, cwd)
// substituted, and captures exactly what got printed.
//
// Every seam Run touches — os.Stdout/os.Stderr, os.Environ, the process cwd,
// runner's prompt funcs — is process-global, so tests in this package must
// never run in parallel (no t.Parallel()). envMu enforces serialization
// against other e2e tests; it does nothing for non-e2e tests sharing the
// process, which is why this package owns its own dedicated test binary.
func Run(t *testing.T, c Case) *Result {
	t.Helper()
	envMu.Lock()
	defer envMu.Unlock()

	files := c.Files
	if files == nil {
		files = Files{"hobnob.yml": c.Yml}
	}
	for _, content := range files {
		if strings.Contains(content, "sleep ") {
			t.Fatal("fixture contains 'sleep ' — e2e tests must not sleep; see harness_test.go runtime rules")
		}
	}

	dir := t.TempDir()
	dir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", dir, err)
	}

	if c.Dir != "" {
		copyTestdataDir(t, c.Dir, dir)
	}
	for relPath, content := range files {
		full := filepath.Join(dir, relPath)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q): %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte(dedent(content)), 0o644); err != nil {
			t.Fatalf("WriteFile(%q): %v", full, err)
		}
	}

	cwd := dir
	if c.Cwd != "" {
		cwd = filepath.Join(dir, c.Cwd)
		if err := os.MkdirAll(cwd, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q): %v", cwd, err)
		}
	}
	t.Chdir(cwd)

	restoreEnv := isolateEnv(t, c.Env)
	defer restoreEnv()

	args := c.Args
	if !c.Discover {
		tfPath := filepath.Join(dir, "hobnob.yml")
		args = append([]string{"--file", tfPath}, args...)
	}

	state := newPromptState(t, c.Answers)
	state.dir, state.probe, state.interrupt = dir, c.Probe, c.Interrupt
	restorePrompts := runner.SetPrompts(state.ask, state.say)
	defer restorePrompts()

	a := app.New("v0.0.0-test")
	a.IsTerminal = func() bool { return c.TTY }
	a.Screen = func() *tui.Screen { return c.Screen }
	restoreStdin := feedStdin(t, c.Stdin)
	defer restoreStdin()
	a.SelectTask = func(ctx context.Context, tasks []tui.TaskItem) (string, error) {
		if c.TaskPick == "" {
			t.Fatalf("unexpected task-select prompt (%d tasks offered) — Case.TaskPick not set", len(tasks))
		}
		state.record(PromptCall{VarName: "(task select)", Task: c.TaskPick})
		return c.TaskPick, nil
	}

	res := &Result{Dir: dir}
	runCtx, cancelRun := context.WithCancel(t.Context())
	defer cancelRun()
	var browsers sync.WaitGroup
	playBrowser := func(url string) {
		browsers.Add(1)
		go func() {
			defer browsers.Done()
			finished := false
			defer func() {
				if !finished {
					cancelRun()
				}
			}()
			browser := newBrowser(t, url, dir)
			browser.interrupt = cancelRun
			c.Browser(browser)
			finished = true
		}()
	}
	a.OpenBrowser = func(url string) error {
		res.URL = url
		if c.Browser == nil {
			t.Errorf("unexpected OpenBrowser(%s) — Case.Browser not set", url)
			cancelRun()
			return nil
		}
		playBrowser(url)
		return nil
	}
	ran := make(chan struct{})
	if port := portArg(args); port != "" && c.Browser != nil {
		// --port opens no browser, so the played browser goes to the port
		// once it answers.
		browsers.Add(1)
		go func() {
			defer browsers.Done()
			if awaitPort(port, ran) {
				playBrowser("http://127.0.0.1:" + port + "/")
			}
		}()
	}
	stdout, stderr, combined := captureOutput(t, func(stdoutNow func() string) {
		var watcher sync.WaitGroup
		if c.WhileRunning != nil {
			watcher.Add(1)
			go func() {
				defer watcher.Done()
				c.WhileRunning(func() []string { return termsim.Render(ansiRe.ReplaceAllString(stdoutNow(), "")) })
			}()
		}
		res.runErr = a.Run(runCtx, args)
		close(ran)
		watcher.Wait()
		browsers.Wait()
	})
	res.Raw = stdout
	if c.Screen != nil {
		stdout = strings.Join(termsim.Render(ansiRe.ReplaceAllString(stdout, "")), "\n")
	}
	res.Stdout = ansiRe.ReplaceAllString(stdout, "")
	res.Stderr = ansiRe.ReplaceAllString(stderr, "")
	res.Combined = ansiRe.ReplaceAllString(combined, "")
	res.Prompts = state.calls
	if res.runErr != nil {
		res.ExitCode = 1
	}
	return res
}

// isolateEnv clears the process environment down to PATH/HOME/TMPDIR plus
// exactly what env specifies, and returns a func restoring the original
// environment. scope.Load reads os.Environ() wholesale, so anything left
// over from the host (CI, USER, a stray VERSION) would otherwise leak into
// scope and silently change a test's result.
// CI is deliberately not restored: defaultNoPrompts() must be driven solely
// by Case.TTY.
func isolateEnv(t *testing.T, env map[string]string) (restore func()) {
	t.Helper()
	setenv := func(key, val string) {
		if err := os.Setenv(key, val); err != nil {
			t.Fatalf("setenv %s: %v", key, err)
		}
	}
	for _, forbidden := range []string{"PATH", "HOME"} {
		if _, ok := env[forbidden]; ok {
			t.Fatalf("Case.Env must not set %s — every scope var is exported into the child shell (see envWithScopeOverrides), so overriding it breaks sh -c itself, not hobnob", forbidden)
		}
	}

	orig := os.Environ()
	origPath, origHome, origTmp := os.Getenv("PATH"), os.Getenv("HOME"), os.Getenv("TMPDIR")

	os.Clearenv()
	setenv("PATH", origPath)
	setenv("HOME", origHome)
	if origTmp != "" {
		setenv("TMPDIR", origTmp)
	}
	for k, v := range env {
		setenv(k, v)
	}

	return func() {
		os.Clearenv()
		for _, kv := range orig {
			if key, val, ok := strings.Cut(kv, "="); ok {
				setenv(key, val)
			}
		}
	}
}

// feedStdin swaps os.Stdin for a pipe holding text, closed at its end, and
// returns a func restoring the original.
func feedStdin(t *testing.T, text string) (restore func()) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	if _, err := w.WriteString(text); err != nil {
		t.Fatalf("write stdin: %v", err)
	}
	w.Close()
	orig := os.Stdin
	os.Stdin = r
	return func() { os.Stdin = orig; r.Close() }
}

// captureOutput swaps os.Stdout/os.Stderr for the duration of f, draining
// both concurrently so output past the ~64KB pipe buffer can't deadlock.
func captureOutput(t *testing.T, f func(stdoutNow func() string)) (stdout, stderr, combined string) {
	t.Helper()
	rOut, wOut, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	rErr, wErr, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	origOut, origErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = wOut, wErr

	var mu sync.Mutex
	var outBuf, errBuf, combinedBuf bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(2)
	drain := func(r *os.File, dst *bytes.Buffer) {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			n, readErr := r.Read(buf)
			if n > 0 {
				mu.Lock()
				dst.Write(buf[:n])
				combinedBuf.Write(buf[:n])
				mu.Unlock()
			}
			if readErr != nil {
				return
			}
		}
	}
	go drain(rOut, &outBuf)
	go drain(rErr, &errBuf)

	f(func() string {
		mu.Lock()
		defer mu.Unlock()
		return outBuf.String()
	})

	os.Stdout, os.Stderr = origOut, origErr
	wOut.Close()
	wErr.Close()
	wg.Wait()
	rOut.Close()
	rErr.Close()

	return outBuf.String(), errBuf.String(), combinedBuf.String()
}

func copyTestdataDir(t *testing.T, name, destRoot string) {
	t.Helper()
	src := filepath.Join("testdata", name)
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatalf("read testdata dir %q: %v", src, err)
	}
	var walk func(rel string)
	walk = func(rel string) {
		full := filepath.Join(src, rel)
		info, err := os.Stat(full)
		if err != nil {
			t.Fatalf("stat %q: %v", full, err)
		}
		if info.IsDir() {
			children, err := os.ReadDir(full)
			if err != nil {
				t.Fatalf("read dir %q: %v", full, err)
			}
			for _, child := range children {
				walk(filepath.Join(rel, child.Name()))
			}
			return
		}
		content, err := os.ReadFile(full)
		if err != nil {
			t.Fatalf("read %q: %v", full, err)
		}
		destPath := filepath.Join(destRoot, rel)
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q): %v", filepath.Dir(destPath), err)
		}
		if err := os.WriteFile(destPath, content, 0o644); err != nil {
			t.Fatalf("write %q: %v", destPath, err)
		}
	}
	for _, entry := range entries {
		walk(entry.Name())
	}
}

// promptState backs the fake prompt implementations installed via
// runner.SetPrompts, plus the fake task-select. It is not itself concurrency
// -safe against Run() calls in other tests — Run's envMu lock already
// prevents that — but the drain goroutines in captureOutput never touch it,
// so no locking is needed for those.
type promptState struct {
	t         *testing.T
	answers   map[string][]string
	askIdx    map[string]int
	calls     []PromptCall
	dir       string   // the temp root, for resolving probe paths
	probe     []string // see Case.Probe
	interrupt string   // see Case.Interrupt
}

func newPromptState(t *testing.T, answers map[string][]string) *promptState {
	return &promptState{
		t:       t,
		answers: answers,
		askIdx:  map[string]int{},
	}
}

func (s *promptState) record(c PromptCall) {
	s.calls = append(s.calls, c)
}

// ask implements runner.AskFunc as a plain answer feeder (a picker's answers are paths, relative ones from the invocation directory): it offers each
// answer for q.VarName to the question's judge in turn, as a person retyping
// after a refusal would, and takes the first one the judge doesn't refuse.
// Menu answers are option names and picker answers paths, comma-separated under multi:. It fails loudly
// rather than hanging if every answer is exhausted without success.
func (s *promptState) ask(ctx context.Context, q ask.Question) (value.Value, error) {
	s.t.Helper()
	if q.VarName == s.interrupt {
		return value.Value{}, tea.ErrProgramKilled
	}
	answers := s.answers[q.VarName]
	var refusals []string
	for i := s.askIdx[q.VarName]; i < len(answers); i++ {
		verdict := q.Judge(answerFrom(q, answers[i]))
		if verdict.Refusal != "" {
			refusals = append(refusals, verdict.Refusal)
			continue
		}
		s.askIdx[q.VarName] = i + 1
		call := PromptCall{VarName: q.VarName, Info: q.Info, Default: q.Default, Items: q.Options, Multi: q.Multi, Secret: q.Secret, Optional: q.Optional, Task: q.Task, Refusals: refusals}
		if q.Path != nil {
			s.describePicker(&call, q.Path)
		}
		s.record(call)
		return verdict.Value, nil
	}
	s.t.Fatalf("unexpected prompt for %q (task %q): no answer was accepted (refusals: %q), or none were provided", q.VarName, q.Task, refusals)
	return value.Value{}, errors.New("unreachable")
}

// describePicker records what a picker question carried: its kind, defaults
// and what its predicates report for each Case.Probe path.
func (s *promptState) describePicker(call *PromptCall, rules *ask.PathRules) {
	call.Type = "file"
	if rules.Dir {
		call.Type = "dir"
	}
	call.Default = strings.Join(rules.Defaults, ",")
	for _, p := range s.probe {
		full := filepath.Join(s.dir, p)
		info, err := os.Stat(full)
		if err != nil {
			s.t.Fatalf("probe path %q: %v", p, err)
		}
		if rules.Included != nil {
			if call.Included == nil {
				call.Included = map[string]bool{}
			}
			call.Included[p] = rules.Included(full, info.IsDir())
		}
		if rules.Excluded != nil {
			if call.Excluded == nil {
				call.Excluded = map[string]bool{}
			}
			call.Excluded[p] = rules.Excluded(full, info.IsDir())
		}
	}
}

func answerFrom(q ask.Question, raw string) ask.Answer {
	if !q.IsMenu() && q.Path == nil {
		return ask.Answer{Text: raw}
	}
	if raw == "" {
		return ask.Answer{}
	}
	if q.Multi {
		return ask.Answer{Picks: strings.Split(raw, ",")}
	}
	return ask.Answer{Picks: []string{raw}}
}

// say implements runner.SayPromptFunc: the person always continues.
func (s *promptState) say(ctx context.Context, message, task string) error {
	s.record(PromptCall{Type: "say", Info: message, Task: task})
	return nil
}
