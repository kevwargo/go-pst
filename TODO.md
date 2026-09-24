## Bugs

### Processes disappear in interactive mode

Sometimes (hard to catch, happened a few times when bash internally executed completion commands)
when `--show-dead` is enabled some processes appear for a fraction of a second and then disappear.

## Development ideas

### Multiline

Render long processes' command lines on multiple lines, while also respecting the tree line/branch
characters.

### Pin to bottom

Add the ability to always keep the process tree scrolled all the way down.

### Custom process format

Allow to pass a custom string as process format, and parse it as Go template.
