## Bugs

### Reparented process is shown multiple times
```
[1] /usr/lib/systemd/systemd --switched-root --system --deserialize=46
  [8521] /usr/lib/systemd/systemd --user --deserialize=10
->  [270026] /usr/bin/python3 ./trainpwd.py Weeoeoe
    [15428] konsole
      [269399] /bin/bash
        [269947]*e:0* /usr/bin/python3 ./trainpwd.py Weeoeoe
->        [270026] /usr/bin/python3 ./trainpwd.py Weeoeoe
```

## Development ideas

### Custom process format

Allow to pass a custom string as process format, and parse it as Go template.
