## dmenu-desktop-go

This is a little program that allows you to launch .desktop files on your system from dmenu and similar programs.
It also supports aliases and exludes through a config and is much better that my previous project [dmenu-desktop-bash](https://github.com/technicfan/dmenu-desktop-bash).

### Info:

It's really not optimized. When you run it the CPU really spices, but it's ok i guess :)<br>
It's pretty fast I'd say, but [j4-dmenu-desktop](https://github.com/enkore/j4-dmenu-desktop) is still faster.
Like j4-dmenu-desktop, it searches `$XDG_DATA_DIRS` and `$XDG_DATA_HOME` for .desktop files (only the `applications/` folders).<br>
I tried to conform to the [Desktop Entry Specification](https://specifications.freedesktop.org/desktop-entry-spec/1.5/) but I don't know if everything works like intended.

### Usage:

Just download the binary or compile it yourself and run it.
All arguments are directly passed to your menu command.
When you input something in your menu that is not in the list, it be run if it's a valid command.

### Config:

The config file is stored and automatically generated at `$XDG_CONFIG_HOME/dmenu-desktop-go/config.json` or `$HOME/.config/dmenu-desktop-go/config.json`.
The default config looks like this (without the examples for aliases and exludes):

```json
{
    "menu_command": "dmenu -i -p Run:",
    "terminal_command": "kitty",
    "aliases": {
        "<your alias>": {
            "command": "<your command>",
            "is_desktop": true/false
        }
    },
    "excludes": [
        "<your exlude>"
    ]
}
```

The menu command is the command that gets the app list from stdin and should return one selected string.<br>
The terminal command is the command that is run if `Terminal=true` is specified in the .desktop file.<br>
The aliases are just that. They override .desktop app entries.
For commands you have to surround arguments with spaces with (escaped because json) double quotes. So you would have to write `command \"path/to/file\"`.
When you set `"is_desktop": true` the .desktop file where the name is equal to the `"command"`-key or a .desktop file directly if given a path.<br>
The excludes are strings that are removed from the final list which includes aliases.
