function __fish_hobnob_file_value
    set -l cmd (commandline -opc)
    for i in (seq 2 (count $cmd))
        if test "$cmd[$i]" = "--file"; and test (math $i + 1) -le (count $cmd)
            echo $cmd[(math $i + 1)]
            return
        else if string match -qr '^--file=(.+)' "$cmd[$i]"
            string replace --regex '^--file=' '' "$cmd[$i]"
            return
        end
    end
end
function __fish_hobnob_no_task_given
    set -l cmd (commandline -opc)
    set -l positional 0
    set -l i 2
    while test $i -le (count $cmd)
        if test "$cmd[$i]" = "--file"
            set i (math $i + 2)
        else if not string match -qr '^--' "$cmd[$i]"; and not string match -qr '=' "$cmd[$i]"
            set positional (math $positional + 1)
            set i (math $i + 1)
        else
            set i (math $i + 1)
        end
    end
    test $positional -eq 0
end
function __fish_hobnob_tasks
    set -l f (__fish_hobnob_file_value)
    if test -n "$f"
        hobnob --file "$f" --list 2>/dev/null | awk 'NR>1 && !/^[[:space:]]/{print $2}'
    else
        hobnob --list 2>/dev/null | awk 'NR>1 && !/^[[:space:]]/{print $2}'
    end
end
complete -c hobnob -l file -r -d 'Hobnob file to use'
complete -c hobnob -f -l demo -d 'Run the built-in demo taskfile'
complete -c hobnob -f -l list -d 'List all available tasks'
complete -c hobnob -f -l select -d 'Interactively select a task to run'
complete -c hobnob -f -l help -d 'Show help'
complete -c hobnob -f -l no-input -d 'Skip interactive prompts'
complete -c hobnob -f -l version -d 'Print version and exit'
complete -c hobnob -f -l upgrade -d 'Upgrade to latest release'
complete -c hobnob -f -n "__fish_hobnob_no_task_given" -a "(__fish_hobnob_tasks)"
