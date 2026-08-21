_hobnob() {
  local cur="${words[CURRENT]}"
  local prev="${words[CURRENT-1]}"

  if [[ "$prev" == "--file" ]]; then
    _files
    return
  fi

  if [[ "$cur" == "--file="* ]]; then
    compset -P '--file='
    _files
    return
  fi

  if [[ "$cur" == --* ]]; then
    compadd -- --file --demo --list --select --help --no-input --version --upgrade
    return
  fi

  local file_arg="" positional=0 i
  for (( i=2; i<CURRENT; i++ )); do
    if [[ "${words[i]}" == "--file" ]]; then
      file_arg="${words[i+1]}"
      (( i++ ))
    elif [[ "${words[i]}" != --* && "${words[i]}" != *=* ]]; then
      (( positional++ ))
    fi
  done

  if (( positional == 0 )); then
    local tasks
    if [[ -n "$file_arg" ]]; then
      tasks=(${(f)"$(hobnob --file "$file_arg" --list 2>/dev/null | awk 'NR>1 && !/^[[:space:]]/{print $2}')"})
    else
      tasks=(${(f)"$(hobnob --list 2>/dev/null | awk 'NR>1 && !/^[[:space:]]/{print $2}')"})
    fi
    compadd -a tasks
  fi
}
type compdef &>/dev/null || { autoload -Uz compinit && compinit; }
compdef _hobnob hobnob
