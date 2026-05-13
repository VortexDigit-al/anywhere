# anywhere-cd.fish — fish shell integration for anywhere.
#
# Installation
# ------------
#   cp anywhere-cd.fish ~/.config/fish/functions/any.fish
#
# Fish auto-loads every file in ~/.config/fish/functions/ by function name,
# so no sourcing or config.fish edit is needed.
#
# How it works
# ------------
# anywhere is launched with --cd-file pointing to a temp file.
# When you press Ctrl+D on a result, anywhere writes that directory to the
# file and exits.  This function reads the file and calls cd.
# Without the wrapper, Ctrl+D opens a new terminal window instead.

function any --description 'anywhere — file search with cd-on-exit'
    set -l _tmp (mktemp)
    anywhere --cd-file $_tmp $argv
    if test -s $_tmp
        cd (string trim -- (cat $_tmp))
    end
    rm -f $_tmp
end
