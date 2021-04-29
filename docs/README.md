We build our documentation using [mkdocs](https://www.mkdocs.org/).

Our live docs site (https://rancher.github.io/opni) is published using a GitHub Action. You can see the configuration of this action in the file `$PROJECT_ROOT/.github/workflows/docs.yaml`.

Additional configuration to make this happen is the `Pages` section of the project's Settings in GitHub.

You can build and view the site locally by running these two commands:
```
docker build -t mkdocs -f Dockerfile.docs .
docker run -p 8000:8000 --rm -it -v ${PWD}:/docs mkdocs serve -a 0.0.0.0:8000
```

Note that the first command is just a one-time step. As long as you dont delete the `mkdocs` image it built, you do not need to run that command again.

After you run the second command, you'll be able to load the site in your browser at `http://0.0.0.0:8000/`

The site content is built based on the following files:
- `Dockerfile.docs` - Just builds the local environment necessary for serving the site locally. It installs the same mkdocs components that `.github/workflows/docs.yaml` does.
- `mkdocs.yml` - Contains site configuration and navigation.
- `docs/` - Directory containing the actual documentation content. Put images in the `assets` subdirectory.
