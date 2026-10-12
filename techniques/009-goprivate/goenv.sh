# Technique 9: the one script ledgerd's CI and onboarding doc
# source (the Dockerfile sets the same values with ENV). Plain
# environment variables, so no machine depends on go env -w.
export GOPRIVATE='git.corp.example.com/*'
export GOPROXY=https://goproxy.corp.example.com,direct
export GOFLAGS='-trimpath -buildvcs=true'
export GIT_TERMINAL_PROMPT=0
