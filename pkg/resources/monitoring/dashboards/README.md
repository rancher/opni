To build dashboards, run:
```bash
$ jb update
$ jsonnet -J vendor -e '(import "dashboards.jsonnet")' > dashboards.json
```