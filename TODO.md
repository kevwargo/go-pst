## Bugs

### Processes disappear in interactive mode

Sometimes (hard to catch, happened a few times when bash internally executed completion commands)
when `--show-dead` is enabled some processes appear for a fraction of a second and then disappear.

## Development ideas

### Custom process format

Allow to pass a custom string as process format, and parse it as Go template.
