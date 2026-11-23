package catalog

import "testing"

// Each case is a tiny repository and the lifecycle pins it must yield.
// Pins outside want are allowed only when listed in also.
func TestParsersYieldPins(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  []pinKey
	}{
		{"github actions setup + matrix + runner", map[string]string{
			".github/workflows/ci.yml": `
jobs:
  test:
    runs-on: ${{ matrix.os }}
    strategy:
      matrix:
        os: [ubuntu-18.04, macos-10.15]
        ubuntu: ["16.04"]
        python-version: [3.6, "3.10"]
        include:
          - python-version: 2.7
    steps:
      - uses: actions/setup-python@v2
        with:
          python-version: ${{ matrix.python-version }}
      - uses: actions/setup-node@v3
        with:
          node-version: 12.x
      - uses: actions/setup-java@v1
        with:
          java-version: 1.8
  other:
    runs-on: ubuntu-${{ matrix.ubuntu }}
    strategy:
      matrix:
        ubuntu: ["16.04"]
    steps:
      - uses: ruby/setup-ruby@v1
        with:
          ruby-version: ruby-2.6.6
`}, []pinKey{{"ubuntu", "16.04"}, {"ruby", "2.6.6"}, {"ubuntu", "18.04"}, {"macos", "10.15"}, {"python", "3.6"}, {"python", "3.10"}, {"python", "2.7"}, {"nodejs", "12"}, {"java", "8"}}},
		{"travis", map[string]string{".travis.yml": `
language: python
dist: trusty
python:
  - "2.7"
  - "3.5"
jdk: oraclejdk8
`}, []pinKey{{"ubuntu", "14.04"}, {"python", "2.7"}, {"python", "3.5"}, {"java", "8"}}},
		{"compose interpolation defaults", map[string]string{"docker-compose.yml": "services:\n  db:\n    image: postgres:${PG_VERSION:-9.6}\n  app:\n    image: \"${APP_IMAGE}\"\n"},
			[]pinKey{{"postgresql", "9.6"}}},
		{"gitlab services", map[string]string{".gitlab-ci.yml": `
image: node:10
services:
  - postgres:9.6
  - name: redis:3.2
`}, []pinKey{{"nodejs", "10"}, {"postgresql", "9.6"}, {"redis", "3.2"}}},
		{"serverless + SAM + GAE", map[string]string{
			"serverless.yml": "provider:\n  name: aws\n  runtime: nodejs8.10\n",
			"template.yaml":  "Resources:\n  Fn:\n    Properties:\n      Runtime: python3.6\n",
			"app.yaml":       "runtime: python27\n",
		}, []pinKey{{"aws-lambda", "nodejs8.10"}, {"aws-lambda", "python3.6"}, {"python", "2.7"}}},
		{"package.json engines and frameworks", map[string]string{"package.json": `{
  "engines": {"node": "^10.13.0"},
  "dependencies": {"vue": "^2.6.11", "@angular/core": "~8.2.14", "express": "^4.17.1", "left-pad": "1.0.0"},
  "devDependencies": {"electron": "9.4.4", "jquery": ">=1.8"}
}`}, []pinKey{{"nodejs", "10"}, {"vue", "2"}, {"angular", "8.2"}, {"express", "4"}, {"electron", "9.4.4"}}},
		{"composer", map[string]string{"composer.json": `{
  "require": {"php": "^7.1.3", "laravel/framework": "5.8.*", "ext-json": "*", "behat/behat": "2.5.*@stable"},
  "require-dev": {"symfony/framework-bundle": "^4.4"}
}`}, []pinKey{{"php", "7"}, {"laravel", "5.8"}, {"symfony", "4"}, {"behat", "2.5"}}},
		{"python manifests", map[string]string{
			"requirements/base.txt":   "Django>=2.2,<3.0  # pinned\nnumpy==1.16.4 ; python_version < '3.8'\nflask\n",
			"Pipfile":                 "[packages]\ndjango = \"==1.11.29\"\n\n[requires]\npython_version = \"3.6\"\n",
			"pyproject.toml":          "[tool.poetry.dependencies]\npython = \"~3.7\"\ndjango = \"^3.1\"\n",
			"environment.yml":         "dependencies:\n  - python=3.6\n  - openssl=1.1.1k\n  - pip:\n    - Django==2.0.13\n    - pyspark==3.1.2\n",
			".pre-commit-config.yaml": "repos:\n  - repo: x\n    hooks:\n      - id: black\n        language_version: python3.6\n",
			"tox.ini":                 "[tox]\nenvlist = py{27,35}-django{111,22}, flake8\n",
		}, []pinKey{{"django", "2"}, {"numpy", "1.16.4"}, {"django", "1.11.29"}, {"python", "3.6"}, {"python", "3.7"},
			{"django", "3"}, {"django", "2.0.13"}, {"python", "2.7"}, {"python", "3.5"}, {"openssl", "1.1.1"}, {"apache-spark", "3.1.2"}}},
		{"gemfile", map[string]string{"Gemfile": "source 'https://rubygems.org'\nruby '2.5.1'\ngem 'rails', '~> 5.2.0'\n"},
			[]pinKey{{"ruby", "2.5.1"}, {"rails", "5.2"}}},
		{"misc platforms", map[string]string{
			"stack.yaml":         "resolver: lts-13.0\n",
			"buildozer.spec":     "[app]\nrequirements = python3==3.7.6,kivy==2.0.0\n",
			"action.yml":         "runs:\n  using: node12\n  main: dist/index.js\n",
			".python-version":    "anaconda3-2020.02\n",
			"docker-compose.yml": "services:\n  db:\n    image: pgvector/pgvector:pg14\n",
			".travis.yml":        "language: ruby\naddons:\n  postgresql: \"9.6\"\n",
			"build.gradle":       "node {\n  version = '4.4.7'\n  download = true\n}\n",
			"package.json":       `{"peerDependencies": {"react-native": "^0.59.5"}}`,
			"pom.xml":            "<project><dependencies><dependency><groupId>org.apache.tomcat</groupId><artifactId>servlet-api</artifactId><version>6.0.37</version></dependency></dependencies></project>",
		}, []pinKey{{"ghc", "8.6"}, {"python", "3.7.6"}, {"postgresql", "9.6"}, {"nodejs", "4.4.7"}, {"react-native", "0.59"}, {"tomcat", "6.0.37"},
			{"nodejs", "12"}, {"python", "3.7"}, {"postgresql", "14"}}},
		{"maven with properties", map[string]string{"pom.xml": `<project>
  <properties>
    <java.version>1.8</java.version>
    <spring.version>4.3.9.RELEASE</spring.version>
  </properties>
  <parent><groupId>org.springframework.boot</groupId><artifactId>spring-boot-starter-parent</artifactId><version>1.5.9.RELEASE</version></parent>
  <dependencies>
    <dependency><groupId>org.springframework</groupId><artifactId>spring-core</artifactId><version>${spring.version}</version></dependency>
    <dependency><groupId>log4j</groupId><artifactId>log4j</artifactId><version>1.2.17</version></dependency>
  </dependencies>
</project>`}, []pinKey{{"java", "8"}, {"spring-framework", "4.3.9.RELEASE"}, {"spring-boot", "1.5.9"}, {"log4j", "1.2.17"}}},
		{"gradle", map[string]string{
			"build.gradle":      "plugins {\n  id 'org.springframework.boot' version '2.1.4.RELEASE'\n}\nsourceCompatibility = 1.8\ndependencies {\n  implementation \"org.jetbrains.kotlin:kotlin-stdlib:$kotlin_version\"\n}\n",
			"gradle.properties": "kotlin_version=1.3.72\n",
		}, []pinKey{{"spring-boot", "2.1.4.RELEASE"}, {"java", "8"}, {"kotlin", "1.3.72"}}},
		{".NET", map[string]string{
			"src/App/App.csproj": "<Project><PropertyGroup><TargetFrameworks>netcoreapp2.1;net461;netstandard2.0</TargetFrameworks></PropertyGroup></Project>",
			"Legacy/Old.csproj":  "<TargetFrameworkVersion>v4.5</TargetFrameworkVersion>",
			"global.json":        `{"sdk": {"version": "3.1.100"}}`,
			"Site/Web.config":    `<httpRuntime targetFramework="4.5.2" />`,
			"Tool/App.config":    `<supportedRuntime version="v4.0" sku=".NETFramework,Version=v4.0" />`,
		}, []pinKey{{"dotnet", "2.1"}, {"dotnetfx", "4.6.1"}, {"dotnetfx", "4.5"}, {"dotnet", "3.1"}, {"dotnetfx", "4.5.2"}, {"dotnetfx", "4.0"}}},
		{"dockerfile ARG defaults and variants", map[string]string{"Dockerfile": `
ARG NODE_VERSION=12
FROM node:${NODE_VERSION}-stretch AS build
FROM maven:3.6.3-jdk-8 AS java
FROM python:3.6-alpine3.12
FROM build
COPY --from=composer:1 /usr/bin/composer /usr/bin/composer
FROM mcr.microsoft.com/dotnet/core/sdk:2.2
FROM mcr.microsoft.com/vscode/devcontainers/python:0-3.7
FROM resin/rpi-raspbian:jessie
FROM node:carbon
FROM gcr.io/distroless/python3-debian10
FROM clojure:temurin-11-alpine
FROM ubuntu:zesty
FROM phusion/baseimage:0.9.15
FROM bde2020/hadoop-namenode:1.1.0-hadoop2.8-java8
FROM registry.access.redhat.com/ubi8/openjdk-11-runtime:1.10
`}, []pinKey{{"java", "11"}, {"ubuntu", "14.04"}, {"apache-hadoop", "2.8"}, {"debian", "8"}, {"nodejs", "8"}, {"debian", "10"}, {"java", "11"}, {"ubuntu", "17.04"}, {"nodejs", "12"}, {"debian", "9"}, {"apache-maven", "3.6.3"}, {"java", "8"}, {"python", "3.6"},
			{"alpine-linux", "3.12"}, {"composer", "1"}, {"dotnet", "2.2"}, {"python", "3.7"}}},
		{"vagrant + jenkins + terraform + version files", map[string]string{
			"Vagrantfile":                        "config.vm.box = \"ubuntu/trusty64\"\n",
			"Jenkinsfile":                        "agent { docker { image 'node:8-alpine' } }\n",
			"main.tf":                            "terraform {\n  required_version = \"~> 0.12.0\"\n}\nresource \"aws_lambda_function\" \"f\" {\n  runtime = \"nodejs10.x\"\n}\n",
			".tool-versions":                     "nodejs 14.17.0\njava adoptopenjdk-11.0.11+9\n",
			".sdkmanrc":                          "java=17.0.2-tem\n",
			".rvmrc":                             "rvm 1.9.3@app --create\n",
			"demo/project.godot":                 "config_version=5\n[application]\nconfig/features=PackedStringArray(\"4.2\", \"Forward Plus\")\n",
			"CMakeLists.txt":                     "find_package(Qt5 COMPONENTS Widgets REQUIRED)\n",
			"build.xml":                          "<project><javac srcdir=\"src\" source=\"1.6\" target=\"1.6\"/></project>",
			"rust-toolchain.toml":                "[toolchain]\nchannel = \"1.56.0\"\n",
			"ProjectSettings/ProjectVersion.txt": "m_EditorVersion: 2019.4.40f1\n",
		}, []pinKey{{"unity", "2019.4.40"}, {"ruby", "1.9.3"}, {"godot", "4.2"}, {"qt", "5"}, {"java", "6"}, {"ubuntu", "14.04"}, {"nodejs", "8"}, {"terraform", "0.12"}, {"aws-lambda", "nodejs10.x"},
			{"nodejs", "14.17.0"}, {"java", "11.0.11"}, {"java", "17.0.2"}, {"rust", "1.56.0"}}},
		{"mixed CI, build args and release downloads", map[string]string{
			".travis.yml":              "language: node_js\nnode_js: 10\njobs:\n  include:\n    - name: x\n\t   before_script: yarn\n",
			"src/dig/Dockerfile":       "ARG BIND9_VERSION=9.16.50\nFROM alpine:3.20\nARG BIND9_VERSION\nRUN wget https://downloads.isc.org/isc/bind9/${BIND9_VERSION}/bind.tar.xz\n",
			".gitlab-ci.yml":           "image: 10.0.0.1:5000/team/ros-kinetic-amr:latest\n",
			".github/workflows/ci.yml": "jobs:\n  b:\n    runs-on: windows-2022\n    steps:\n      - uses: jurplel/install-qt-action@v4\n        with:\n          version: '5.15.2'\n      - run: curl -LO https://download.firedaemon.com/FireDaemon-OpenSSL/openssl-1.1.1w.zip\n",
			"app/src/main/dist/ansible/roles/node/defaults/main.yml": "nodejs_version: \"10.x\"\n",
			"k8s/grafana.yml":       "image: k8s.gcr.io/heapster-grafana-amd64:v4.4.3\n",
			"win/appveyor.yml":      "image:\n  - Visual Studio 2017\nenvironment:\n  matrix:\n    - APPVEYOR_BUILD_WORKER_IMAGE: Visual Studio 2013\n    - APPVEYOR_BUILD_WORKER_IMAGE: Ubuntu1604\n",
			"lib/CMakeLists.txt":    "find_package(Lua51 REQUIRED)\n",
			"lua/Makefile":          "CFLAGS=$(shell pkg-config --cflags lua5.3)\n",
			"docker/DF_tools_1.0.1": "# test\n\nFROM debian:buster-slim\nARG   go_version=1.7.6\nENV   GO_VERSION=${go_version}\n",
		}, []pinKey{{"debian", "10"}, {"go", "1.7.6"}, {"visual-studio", "15"}, {"visual-studio", "12.0"}, {"ubuntu", "16.04"}, {"lua", "5.1"}, {"lua", "5.3"}, {"nodejs", "10"}, {"bind-9", "9.16.50"}, {"ros", "kinetic"}, {"qt", "5.15.2"}, {"openssl", "1.1.1"}, {"grafana", "4.4.3"}}},
		{"version loops, wheel selectors and toolchain files", map[string]string{
			".docker/workspace/Dockerfile": "FROM ubuntu:24.04\nRUN curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.39.0/install.sh | bash && \\\n    for version in 18 20 22; do \\\n        nvm install $version; \\\n    done\n",
			"appimage/app.yml":             "app: x\n\ningredients:\n  packages:\n    - libfuse\n  dist: trusty\n",
			"airflow/Dockerfile":           "FROM puckel/docker-airflow:1.10.1\n",
			"pinned-toolchain.toml":        "[toolchain]\nchannel = \"1.97.1\"\n",
			".travis.yml":                  "language: node_js\naddons:\n  firefox: '51.0'\n",
			"appveyor.yml":                 "environment:\n  matrix:\n    - PYTHON: \"C:\\\\Python35-x64\"\n    - PYTHON: C:\\Python27\n",
			"pyproject.toml":               "[tool.cibuildwheel]\nbuild = \"cp3{10,11}-*\"\n",
			"setup.sh":                     "wget https://download.virtualbox.org/virtualbox/5.1.12/virtualbox-5.1_5.1.12-112440~Ubuntu~xenial_amd64.deb\nwget http://kernel.ubuntu.com/~kernel-ppa/mainline/v4.10.11/linux-image.deb\n",
			"Makefile":                     "ETCD_VERSIONS := 3.2.32 3.3.27\n",
			".github/workflows/qa.yml":     "jobs:\n  t:\n    runs-on: ubuntu-24.04\n    strategy:\n      matrix:\n        etcd-version: ['3.4.45']\n    env:\n      ETCD_VERSION: ${{ matrix.etcd-version }}\n",
			".moon/toolchains.yml":         "rust:\n  version: 1.99.0\n",
		}, []pinKey{{"nvm", "0.39.0"}, {"nodejs", "18"}, {"nodejs", "20"}, {"ubuntu", "14.04"}, {"apache-airflow", "1.10.1"}, {"rust", "1.97.1"}, {"firefox", "51.0"},
			{"python", "3.5"}, {"python", "2.7"}, {"python", "3.10"}, {"python", "3.11"}, {"virtualbox", "5.1"}, {"linux", "4.10"}, {"etcd", "3.2.32"}, {"etcd", "3.3.27"}, {"etcd", "3.4.45"}, {"rust", "1.99.0"}}},
		{"rbenv, capistrano, k3s and stackage", map[string]string{
			"install_ruby.sh":         "rbenv install -s 2.1.2\nrbenv global 2.1.2\n",
			"config/deploy.rb":        "set :rbenv_ruby, '2.2.2'\n",
			"runtime.txt":             "3.6\n",
			"k3s-bootstrap.sh":        "export K8SVERSION=v1.23.3+k3s1\nk3sup install --k3s-version $K8SVERSION\n",
			"manifests/plan.yml":      "spec:\n  upgrade:\n    image: rancher/k3s-upgrade\n  version: v1.30.2+k3s1\n",
			"stack.yaml":              "resolver:\n  url: https://raw.githubusercontent.com/commercialhaskell/stackage-snapshots/master/lts/20/20.yaml\n",
			"scripts/install-node.sh": "NODE_VERSION=14.17.0\ncurl -O https://example.org/node-v${NODE_VERSION}.tgz\n",
		}, []pinKey{{"ruby", "2.1.2"}, {"ruby", "2.2.2"}, {"python", "3.6"}, {"kubernetes", "1.23"}, {"kubernetes", "1.30"}, {"ghc", "9.2"}, {"nodejs", "14.17.0"}}},
		{"jruby, azure images, sbt and third-party PPAs", map[string]string{
			".github/workflows/ci.yml":  "jobs:\n  t:\n    runs-on: ubuntu-24.04\n    strategy:\n      matrix:\n        ruby: [jruby-9.3, '3.3']\n    steps:\n      - uses: ruby/setup-ruby@v1\n        with:\n          ruby-version: ${{ matrix.ruby }}\n",
			"azure-pipelines.yml":       "pool:\n  vmImage: 'xcode9-macos10.13'\n",
			"component.json":            `{"dependencies": {"jquery": "~1.8.0"}}`,
			"ansible/system.yml":        "- command: subscription-manager repos --enable=rhel-7-server-rpms --enable=rhel-7-server-ose-3.11-rpms\n",
			"examples/npm_install.sh":   "export npm_config_target=4.0.1\nexport npm_config_runtime=electron\nnpm install\n",
			"src/build.sbt":             "libraryDependencies ++= {\n  val sparkVer = \"2.1.0\"\n  Seq(\"org.apache.spark\" %% \"spark-core\" % sparkVer)\n}\n",
			"gradle/libs.versions.toml": "[versions]\njdk = \"23\"\n",
			"docker/Dockerfile":         "FROM ubuntu:18.04\nRUN add-apt-repository ppa:ondrej/php && apt-get install -y php7.2 php7.2-fpm\n",
			"tools/get-spark.sh":        "curl -O https://archive.apache.org/dist/spark/spark-2.3.0/spark-2.3.0-bin-hadoop2.7.tgz\n",
		}, []pinKey{{"jruby", "9.3"}, {"macos", "10.13"}, {"jquery", "1.8"}, {"rhel", "7"}, {"red-hat-openshift", "3.11"}, {"electron", "4.0.1"},
			{"apache-spark", "2.1.0"}, {"java", "23"}, {"php", "7.2"}, {"apache-spark", "2.3.0"}}},
		{"model IDs, platform runtimes and build-tool pins", map[string]string{
			"docker/Dockerfile":                 "FROM nvidia/cuda:10.2-devel-ubuntu18.04\nRUN PIP_INSTALL=\"python -m pip --no-cache-dir install\" && \\\n    $PIP_INSTALL numpy==1.19.3 llvmlite numba\nRUN pip install --user torch==1.6 django==2.2.28\n",
			"config/models/claude37sonnet.yaml": "api_name: \"claude-3-7-sonnet-20250219\"\n",
			"app/llm.py":                        "client.messages.create(model='claude-3-5-sonnet-20240620')\n",
			"gradle/libs.versions.toml":         "[versions]\nserializationPlugin = \"2.0.10\"\nspring = \"5.2.0.RELEASE\"\n[libraries]\nspring-core = { module = \"org.springframework:spring-core\", version.ref = \"spring\" }\n[plugins]\nkotlin-serialization = { id = \"org.jetbrains.kotlin.plugin.serialization\", version.ref = \"serializationPlugin\" }\n",
			"wercker.yml":                       "box: ruby:2.3\n",
			"mygem.gemspec":                     "spec.required_ruby_version = '~>2.3'\n",
			".config/molecule/config.yml":       "platforms:\n  - name: debian10\n    image: docker.io/geerlingguy/docker-debian10-ansible:latest\n  - name: u\n    image: geerlingguy/docker-ubuntu2004-ansible:latest\n",
			"ansible/s2i.yml":                   "builder_image: centos/python-35-centos7:latest\nnode_image: registry.access.redhat.com/rhscl/nodejs-10-rhel7:latest\n",
			"cr.yml":                            "steps:\n  - with:\n      # model: claude-opus-4-20250514\n      model: 'claude-3-haiku-20240307'\n",
			".github/workflows/django.yml":      "jobs:\n  t:\n    runs-on: ubuntu-24.04\n    strategy:\n      matrix:\n        django: ['3.0', '4.1']\n    steps:\n      - run: pip install Django~=${{ matrix.django }}\n",
			"elixir/Dockerfile":                 "ARG DEBIAN_VERSION=bullseye-20240130-slim\nARG RUNNER_IMAGE=\"debian:${DEBIAN_VERSION}\"\nFROM ${RUNNER_IMAGE}\n",
			"roles/demo/tasks/main.yml":         "- shell: \"{{ oc_cmd }} new-app -n {{ p }} centos/python-35-centos7~https://github.com/x/y --name=d\"\n",
			"docker/dev/Dockerfile":             "FROM ubuntu:18.04\nRUN add-apt-repository -y ppa:ondrej/php && \\\n    apt-get install -y \\\n        php7.1 \\\n        php7.1-fpm\n",
			"tox-travis/.travis.yml":            "language: python\nenv:\n  - TOXENV=py34\n",
			"lib/counter_service.ts":            "new lambda.Function(this, 'f', { runtime: lambda.Runtime.NODEJS_10_X })\n",
			"setup.sql":                         "create function f() returns string language python runtime_version = '3.8' handler='h'\n",
			"libs/build_qt_static.bat":          "set QT_VERSION=5.12.3\ncurl -O https://download.qt.io/archive/qt/%QT_VERSION%/single/x.zip\n",
			"definitions/15.0-x86_64.json":      `{"builders":[{"iso_url":"http://download.opensuse.org/distribution/leap/15.0/iso/openSUSE-Leap-15.0-NET-x86_64.iso"}]}`,
			"Compress/Compress.csproj":          "<Project><PropertyGroup><TargetFrameworkVersion>v4.0</TargetFrameworkVersion><TargetFrameworkProfile>Profile92</TargetFrameworkProfile></PropertyGroup></Project>",
			"db/Db.sqlproj":                     "<Project><PropertyGroup><DSP>Microsoft.Data.Tools.Schema.Sql.Sql130DatabaseSchemaProvider</DSP></PropertyGroup></Project>",
			"war/pom.xml":                       "<project><packaging>war</packaging><!-- <artifactId>tomcat6-maven-plugin</artifactId> --><build><plugins><plugin><groupId>org.apache.tomcat.maven</groupId><artifactId>tomcat7-maven-plugin</artifactId><version>2.1</version></plugin></plugins></build></project>",
		}, []pinKey{{"php", "7.1"}, {"python", "3.4"}, {"aws-lambda", "nodejs10.x"}, {"python", "3.8"}, {"qt", "5.12.3"}, {"opensuse", "15.0"}, {"debian", "11"}, {"python", "3.5"}, {"centos", "7"}, {"nodejs", "10"}, {"claude", "claude-3-haiku-20240307"}, {"django", "3"}, {"django", "4"}, {"kotlin", "2.0.10"}, {"spring-framework", "5.2.0"}, {"ruby", "2.3"}, {"ruby", "2"}, {"debian", "10"}, {"ubuntu", "20.04"}, {"mssqlserver", "13.0-sp3"}, {"tomcat", "7"}, {"numpy", "1.19.3"}, {"django", "2.2.28"}, {"claude", "claude-3-7-sonnet-20250219"}, {"claude", "claude-3-5-sonnet-20240620"}}},
		{"ansible roles, IDE metadata and script installs", map[string]string{
			"node/Dockerfile":                   "FROM mhart/alpine-node:8\n",
			".github/workflows/container.yml":   "jobs:\n  t:\n    container: ghcr.io/example/ci/py3.10-app:latest\n",
			"kind/setup.sh":                     "cat <<EOF >/tmp/kind.yaml\n  - role: control-plane\n    image: kindest/node:v1.33.1\nEOF\n",
			"slave/Dockerfile":                  "FROM registry.access.redhat.com/dotnet/dotnet-20-jenkins-slave-rhel7\n",
			".travis.yml":                       "env:\n  matrix:\n    - python=3.6 CONDA_PY=36\n    - DJANGO=Django==1.6.1\n    - DJANGO=https://www.djangoproject.com/download/1.7c1/tarball/\nscript:\n  - conda create -n t python=$python\n  - pip install $DJANGO\n",
			".github/workflows/t.yml":           "jobs:\n  b:\n    runs-on: ${{ steps.platform.outputs.platform }}\n    steps:\n      - run: echo \"platform=macos-12\" >> $GITHUB_OUTPUT\n",
			".pre-commit-config.yaml":           "repos:\n  - repo: local\n    hooks:\n      - id: x\n        language: node\n        additional_dependencies:\n          - \"eslint@9.35.0\"\n",
			"update_bootstrap.sh":               "rm -rf node_modules\nnpm install bootstrap@4 jquery@3\n",
			"hpc/scripts/large_scale.sh":        "#Load module\nmodule load python/3.8 scipy-stack\n",
			"lambda/Makefile":                   "build-dependencies:\n\tpython3.9 -m pip install -r requirements.txt -t .target/ --upgrade\n",
			"services/rds/deploy.sh":            "DB_ENGINE=\"mysql\"\nENGINE_VERSION=\"8.0.43\"\naws rds create-db-instance --engine $DB_ENGINE --engine-version $ENGINE_VERSION\n",
			"tool/versions.env":                 "RUST_VERSION=\"1.93\"\n",
			"roles/liquibase/defaults/main.yml": "liquibase_ver: '3.1.1'\n",
			"roles/splunk/defaults/main.yml":    "splunk_installer: {file: 'splunkforwarder-6.0.1-189883-linux-2.6-amd64.deb', target: ''}\n",
			"roles/debian/defaults/main.yml":    "debian_distro: 'wheezy'\n",
			".idea/misc.xml":                    "<component name=\"ProjectRootManager\" version=\"2\"><output url=\"file://$PROJECT_DIR$/out\" /></component>\n",
			"app/.idea/misc.xml":                "<component name=\"ProjectRootManager\" languageLevel=\"JDK_1_6\" project-jdk-name=\"9\" />\n",
			"pyidea/.idea/misc.xml":             "<component name=\"ProjectRootManager\" version=\"2\" project-jdk-name=\"Python 3.7\" project-jdk-type=\"Python SDK\" />\n",
			"native/CMakeLists.txt":             "find_package(PythonLibs 2.7)\n",
			"qtapp/CMakeLists.txt":              "if(UNIX AND NOT APPLE)\n    set(CMAKE_PREFIX_PATH \"/opt/Qt/Qt.6.11.1-static\" CACHE PATH \"\")\nendif()\n",
		}, []pinKey{{"nodejs", "8"}, {"python", "3.10"}, {"kubernetes", "1.33.1"}, {"dotnet", "2.0"}, {"rhel", "7"},
			{"python", "3.6"}, {"django", "1.6.1"}, {"django", "1.7"}, {"macos", "12"}, {"eslint", "9.35.0"}, {"bootstrap", "4"},
			{"python", "3.8"}, {"python", "3.9"}, {"mysql", "8.0.43"}, {"rust", "1.93"}, {"liquibase", "3.1.1"}, {"splunk", "6.0.1"},
			{"debian", "7"}, {"java", "6"}, {"java", "9"}, {"python", "2.7"}, {"qt", "6.11.1"}}},
		{"orbs, sbt minikube, ember-try and tox braces", map[string]string{
			".circleci/config.yml":      "version: 2.1\norbs:\n    node: 'circleci/node@5.0.0'\nworkflows:\n    build-and-test:\n        jobs:\n            - node/test:\n                  version: 14.18.3\n",
			"k8s-client/build.sbt":      "githubWorkflowAddedJobs += workflowJobMinikube(\"integration-kubernetes-v1-19\", k8sServerVersion = \"v1.19.11\")\n",
			"scala/project/Build.scala": "scalacOptions ++= Seq(\"-target:jvm-1.6\")\n",
			"plugin/pom.xml":            "<project><properties><jenkins.baseline>2.541</jenkins.baseline></properties></project>",
			"env/enviroment.yaml":       "name: dd\nchannels:\n  - conda-forge\ndependencies:\n  - ros-conda-mutex=1.0=melodic\n",
			"worker/appveyor.yml":       "environment:\n  matrix:\n    - stack: go 1.13\n",
			"config/ember-try.js":       "module.exports = { scenarios: [ { name: 'ember-lts-3.13', npm: { devDependencies: { 'ember-source': '~3.13.0' } } } ] };\n",
			"logstash/Dockerfile":       "FROM docker.elastic.co/logstash/logstash-oss:7.13.0-amd64\n",
			"tox/tox.ini":               "[tox]\nenvlist = py{3.5,3.6},pep8\n\n[testenv]\nbasepython =\n    py3.5: python3.5\n",
			"net/App.csproj":            "<Project><PropertyGroup><TargetFrameworks Condition=\"'$(BuildCoreOnly)' != 'True'\">netstandard2.1;net452</TargetFrameworks></PropertyGroup></Project>",
		}, []pinKey{{"nodejs", "14.18.3"}, {"kubernetes", "1.19"}, {"java", "6"},
			{"ros", "melodic"}, {"go", "1.13"}, {"emberjs", "3.13"}, {"logstash", "7.13.0"},
			{"python", "3.5"}, {"python", "3.6"}, {"dotnetfx", "4.5.2"}}},
		{"codenames, Windows builds and misspelled requirements", map[string]string{
			"conda/Dockerfile":         "FROM continuumio/miniconda2:latest\n",
			".travis.yml":              "language: node_js\nnode_js:\n  - lts/carbon\n  - lts/dubnium\nenv:\n  matrix:\n    - DJANGO=\"django >=1.8,<1.9\"\nscript:\n  - pip install \"$DJANGO\"\n",
			".github/workflows/uv.yml": "jobs:\n  b:\n    runs-on: ubuntu-24.04\n    steps:\n      - run: uv run --python 3.8 --resolution lowest-direct -m unittest\n",
			"db/Central.sqlproj":       "<Project><PropertyGroup><TargetFrameworkVersion>v4.5.2</TargetFrameworkVersion></PropertyGroup></Project>",
			"msvc/wrappers/msvcenv.sh": "export VSCMD_VER=16.9.0\nwine cmd /c call vcvarsall.bat $VSCMD_VER\n",
			"nano/Dockerfile":          "FROM microsoft/nanoserver:10.0.14393.576\n",
			"build/Dockerfile":         "RUN export GIT_ROOT=git-2.30.0 && ./build-git.sh $GIT_ROOT\n",
			"font/font_download.sh":    "fontawesomeUrl=\"http://fontawesome.io/assets/font-awesome-4.7.0.zip\"\ncurl -O $fontawesomeUrl\n",
			".github/workflows/rc.yml": "jobs:\n  r:\n    uses: example/actions/.github/workflows/lint.yml@main\n    with:\n      php: '8.1'\n",
			"operator/go.mod":          "module example.com/op\n\ngo 1.21\n\nrequire (\n\tk8s.io/api v0.23.0\n\tsigs.k8s.io/controller-runtime v0.11.0\n)\n",
			"war/pom.xml":              "<project><dependencies><dependency><groupId>org.webjars</groupId><artifactId>bootstrap</artifactId><version>4.0.0</version></dependency></dependencies></project>",
			"requirments.txt":          "torch==1.7.1\nnumpy==1.17.2\nscipy==1.7.3\n",
		}, []pinKey{{"python", "2.7"}, {"nodejs", "8"}, {"nodejs", "10"}, {"django", "1.8"}, {"python", "3.8"},
			{"dotnetfx", "4.5.2"}, {"visual-studio", "16.9.0"}, {"windows-nano-server", "1607"}, {"git", "2.30.0"},
			{"font-awesome", "4.7.0"}, {"php", "8.1"}, {"bootstrap", "4.0.0"}, {"numpy", "1.17.2"}}},
	}
	for _, c := range cases {
		got := pinSet(walk(c.files))
		for _, w := range c.want {
			if !got[w] {
				t.Errorf("%s: missing pin %v (got %v)", c.name, w, got)
			}
		}
	}
}

// A repo-root rust-toolchain file outranks the rustup default CI actions set.
func TestRustToolchainFileShadowsCI(t *testing.T) {
	inv := walk(map[string]string{
		"rust-toolchain.toml":      "[toolchain]\nchannel = \"1.87.0\"\n",
		".github/workflows/ci.yml": "jobs:\n  c:\n    runs-on: ubuntu-24.04\n    steps:\n      - uses: dtolnay/rust-toolchain@1.84.0\n",
	})
	got := pinSet(inv)
	if got[pinKey{"rust", "1.84.0"}] || !got[pinKey{"rust", "1.87.0"}] {
		t.Fatalf("toolchain file must win: %v", got)
	}
}

// Floors and open ranges declare no release line: no pin.
func TestFloorsYieldNoPins(t *testing.T) {
	inv := walk(map[string]string{
		"pricing.py":         `PRICES = {"gpt-3.5-turbo": 1, "gpt-4-0613": 2, "gpt-4-32k": 3, "claude-2.1": 4, "claude-instant-1.2": 5}`,
		"ci.sh":              "pip install 'django>=2.2' numpy\n",
		"scripts/release.sh": "REQUIRED_NODE_VERSION=18.0.0\nnode -v | check $REQUIRED_NODE_VERSION\n",
		"Makefile":           "MIN_GO_VERSION := 1.16\ncheck:\n\tgo-version-check $(MIN_GO_VERSION)\n",
		"package.json":       `{"engines": {"node": ">=8"}, "dependencies": {"vue": "*", "express": "latest"}}`,
		"setup.py":           "setup(python_requires='>=3.6', install_requires=['django>=2.2'])\n",
		"composer.json":      `{"require": {"php": ">=7.2"}}`,
		"pyproject.toml":     "[project]\nrequires-python = \">=3.7\"\n",
	})
	if len(inv.Pins) != 0 {
		t.Fatalf("floors must not pin: %+v", inv.Pins)
	}
}
