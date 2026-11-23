# Supported files

A repo scan reads the files git tracks. It skips vendored code,
submodules, dependency folders and test fixtures.

- **Container images:** Dockerfile `FROM` (with ARG and ENV defaults
  filled in) and `COPY --from`, compose, Kubernetes, Helm values
  (including separate repository and tag keys), CI images and services,
  devcontainer, Jenkinsfile, WDL, Nextflow and CWL. The OS in a tag counts
  too: `node:12-stretch` runs Debian 9.
- **Version files:** `.nvmrc`, `.node-version`, `.python-version`,
  `.ruby-version`, `.rvmrc`, `.tool-versions`, `mise.toml`, `.sdkmanrc`,
  `.java-version`, `runtime.txt`, `global.json`, `rust-toolchain`, Unity
  `ProjectVersion.txt`, `project.godot`.
- **CI:** GitHub Actions `setup-*` actions (matrix and env values filled
  in) and runner labels, Travis, GitLab, CircleCI (including orb job
  versions), Azure Pipelines, AppVeyor images and stacks, tox, CodeBuild,
  Read the Docs, cibuildwheel and moon toolchains. Also install scripts
  (nvm, pyenv, rbenv, rvm, conda, nodesource, third-party PPAs, uv,
  release downloads, k3s) and `*_VERSION` variables that a step uses.
- **Platforms:** Lambda runtimes (Serverless, SAM, CloudFormation,
  Terraform, CDK code), Snowflake Python UDFs, Packer install ISOs, App
  Engine, Platform.sh, RDS and ElastiCache engines, EKS, Vagrant and Test
  Kitchen boxes, snap bases, NixOS channels.
- **Languages and dependencies:** `package.json` (engines, volta,
  dependencies), Composer, requirements, Pipfile, pyproject, setup.py and
  conda, Gemfile and Gemfile.lock, `go.mod`, `pom.xml` (properties and
  parent POMs filled in), Gradle (plugins, toolchains, ext variables,
  resolutionStrategy, version catalogs, wrapper), Maven wrapper, .NET
  projects, `packages.config`, `web.config`, sqlproj, bower, CMake
  (Python, Qt, Lua), gemspec `required_ruby_version`, Terraform
  `required_version`, pre-commit `additional_dependencies`, ember-try
  scenarios, webjars, and `npm install pkg@ver` in scripts.
- **AI model IDs:** dated Claude and OpenAI model IDs written into code
  and config.

## Container images and SBOMs

For a container image, syft lists the packages and the OS release. zephy
also reads the image name and tag, because some products never show up in
an SBOM. RabbitMQ is one: it ships inside an Erlang release.

SBOM files can be Syft JSON, SPDX 2.x JSON or CycloneDX JSON.
