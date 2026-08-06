# Tygnon

Tygnon is a golang tools used to bump versions of [homebrew](https://brew.sh/) Formulas.

Given a directory, it parses the formula to check whether a new version exists (based on tags and/or release).

If a new version, it will update the version formula, as well as the checksum (sha256)

> Note: it can only handle a formula targeting a branch
> In this case, the version number will be `[NB_COMMITS].[HASH_LAST_COMMIT]`

## Usage
```shell
$ tygnon -h
Usage: tygnon [<path> ...] [flags]

Application used to bump releases on internal brew tap

Arguments:
[<path> ...]    path to project directory

Flags:
-h, --help                   Show context-sensitive help.
-T, --token=KEY=VALUE;...    Personal access token ($TYGNON_TOKEN)
-v, --verbose                verbose output ($TYGNON_VERBOSE)
-f, --force                  force overwriting formulas ($TYGNON_FORCE)
-i, --[no-]interactive       interactive mode ($TYGNON_INTERACTIVE)
--no-push                disable git push ($TYGNON_NO_PUSH)
--generate-config        generate config file ($TYGNON_GENERATE_CONFIG)
--run-hooks              run configured hooks after updating formulas ($TYGNON_RUN_HOOKS)
```

## Configuration file

`tygnon` will load the first configuration file encountered in:
- Current directory
- `$HOME/.config/tygnon/tygnon.yaml`
- `$HOME/.config/tygnon/tygnon.yaml`

A configuration file example:
```yaml
# force overwriting formulas
force: false

# generate config file
generate-config: false

# interactive mode
interactive: true

# disable git push
no-push: false

# Personal access token
token:
  gitlab.com: GL_PERSONAL_ACCESS_TOKEN
  github.com: GH_PERSONAL_ACCESS_TOKEN
  git.example.com: GL_EXAMPLE_PERSONAL_ACCESS_TOKEN

# verbose output
verbose: 0

hooks:
  - name: Test
    path: Path
    parameters:
      - param1
      - param2
    formulas:
      - Formula1
      - Formula2
  - name: Test2
    path: Path2
    parameters:
      - param1
      - param2
    formulas:
      - Formula1
      - Formula2
```

## Hooks

Hooks are scripts allowing to modify a formula when a new version exists (update the build with the last commit for instance)

Hooks are configured in the configuration file
```yaml
hooks:
  - name: HookName
    path: /path/to/the/hook/script/file
    parameters:
      - param2
      - param3
    formulas:
      - myFormula1
      - myFormula2
```

> Note: the first parameter given to the hook will always be the absolute path of the formula 


> Note: hooks are executed in the order they are defined in the configuration file

> Note: configuring hooks is not enough to run them - pass `--run-hooks` (or set `run-hooks: true` in the config file) to actually execute them. This is opt-in because hooks run arbitrary scripts, and `tygnon` may load a configuration file automatically from the current directory.