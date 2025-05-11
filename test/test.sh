#!/usr/bin/env sh

cd "$(dirname "$0")"

[ -d "output" ] && rm -r "output"
mkdir output
mkdir -p run

TEST_IP="187.2.2.1"

# Set environment variable defaults
TRAEFIK_WHOAMI_VERSION=${TRAEFIK_WHOAMI_VERSION:=latest}
TRAEFIK_VERSION=${TRAEFIK_VERSION:=latest}

# Go module name
MODULE_NAME=$(grep '^module ' ../go.mod | awk '{print $2}')

TRAEFIK_CONFIG="./run/traefik.toml"
TRAEFIK_CONFIG_BASE="./traefik.base.toml"

DOCKER_COMPOSE="./run/docker-compose.yml"
DOCKER_COMPOSE_BASE="./docker-compose.base.yml"

echo "Using Traefik whoami ${TRAEFIK_WHOAMI_VERSION}"
echo "Using Traefik ${TRAEFIK_VERSION}"

# docker pull traefik/whoami:${TRAEFIK_WHOAMI_VERSION}>/dev/null
# docker pull traefik:${TRAEFIK_VERSION}>/dev/null

cp $TRAEFIK_CONFIG_BASE $TRAEFIK_CONFIG
cp $DOCKER_COMPOSE_BASE $DOCKER_COMPOSE

if [ ${ENVIRONMENT:=development} = "production" ]; then
	# Get latest tag
	LATEST_PLUGIN_VERSION=$(git describe --match "v*" --abbrev=0 --tags HEAD)
	echo "Testing production version ${LATEST_PLUGIN_VERSION}"
	
	cat >> $TRAEFIK_CONFIG <<-EOF
	
	[experimental]
	  [experimental.plugins]
	    [experimental.plugins.cloudflarewarp]
	      moduleName = "${MODULE_NAME}"
	      version = "${LATEST_PLUGIN_VERSION}"
	EOF
else
	echo "Testing development version"
	cat >> $TRAEFIK_CONFIG <<-EOF
	
	[experimental.localPlugins.cloudflarewarp]
	  moduleName = "${MODULE_NAME}"
	EOF

	cat >> $DOCKER_COMPOSE <<-EOF
      - ../../:/plugins-local/src/${MODULE_NAME}:ro
	EOF
fi

checkExit() {
	if [ ! $1 = 0 ]; then
		echo "Test fail"
		sh test.base.sh CLEANUP
		exit 1
	fi
}

"$SHELL" test.base.sh success toml $TEST_IP
checkExit $?
"$SHELL" test.verify.sh success $TEST_IP
EXIT=$?
mv run/logs output/success-toml
checkExit $EXIT

"$SHELL" test.base.sh fail toml $TEST_IP
checkExit $?
"$SHELL" test.verify.sh fail $TEST_IP
EXIT=$?
mv run/logs output/fail-toml
checkExit $EXIT

"$SHELL" test.base.sh invalid toml "1522.20.2"
checkExit $?
"$SHELL" test.verify.sh invalid "1522.20.2"
EXIT=$?
mv run/logs output/invalid-toml
checkExit $EXIT

"$SHELL" test.base.sh success yml $TEST_IP
checkExit $?
"$SHELL" test.verify.sh success $TEST_IP
EXIT=$?
mv run/logs output/success-yml
checkExit $EXIT

"$SHELL" test.base.sh fail yml $TEST_IP
checkExit $?
"$SHELL" test.verify.sh fail $TEST_IP
EXIT=$?
mv run/logs output/fail-yml
checkExit $EXIT

"$SHELL" test.base.sh invalid yml "1522.20.2"
checkExit $?
"$SHELL" test.verify.sh invalid "1522.20.2"
EXIT=$?
mv run/logs output/invalid-yml
checkExit $EXIT

"$SHELL" test.base.sh CLEANUP
