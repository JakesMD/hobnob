_hobnob_completion() {
  local cur="${COMP_WORDS[COMP_CWORD]}"
  local prev="${COMP_WORDS[COMP_CWORD-1]}"

  if [[ "$prev" == "--file" ]]; then
    COMPREPLY=($(compgen -f -- "$cur"))
    return
  fi

  if [[ "$cur" == "--file="* ]]; then
    local val="${cur#--file=}"
    local files=($(compgen -f -- "$val"))
    COMPREPLY=("${files[@]/#/--file=}")
    return
  fi

  if [[ "$cur" == --* ]]; then
    COMPREPLY=($(compgen -W "--file --list --select --help --no-input --version --upgrade" -- "$cur"))
    return
  fi

  local file_arg="" positional=0 i
  for (( i=1; i<COMP_CWORD; i++ )); do
    if [[ "${COMP_WORDS[i]}" == "--file" ]]; then
      file_arg="${COMP_WORDS[i+1]}"
      (( i++ ))
    elif [[ "${COMP_WORDS[i]}" != --* && "${COMP_WORDS[i]}" != *=* ]]; then
      (( positional++ ))
    fi
  done

  if [[ "$positional" -eq 0 ]]; then
    local tasks
    if [[ -n "$file_arg" ]]; then
      tasks=$(hobnob --file "$file_arg" --list 2>/dev/null | awk 'NR>1 && !/^[[:space:]]/{print $2}')
    else
      tasks=$(hobnob --list 2>/dev/null | awk 'NR>1 && !/^[[:space:]]/{print $2}')
    fi
    COMPREPLY=($(compgen -W "${tasks}" -- "${cur}"))
  fi
}
complete -F _hobnob_completion hobnob
