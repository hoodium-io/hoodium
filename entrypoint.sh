#!/bin/sh

#
# This script initializes the Runed node configuration and keyring.
#

set -o errexit # Exit on error

# Global variables
CLIENT_CONFIG_FILE="${RUNED_HOME}/config/client.toml"
APP_CONFIG_FILE="${RUNED_HOME}/config/app.toml"
CONFIG_FILE="${RUNED_HOME}/config/config.toml"

init_keyring() {
  test -f "${RUNED_HOME}/keyring-file/keyhash" && {
    echo "Keyring already exists!"
    return
  }

  echo "Prepare keyring..."
  (echo "${KEYRING_MNEMONIC}"; echo "${KEYRING_PASSWORD}"; echo "${KEYRING_PASSWORD}") \
    | runed keys add \
      "${KEYRING_NAME}" \
      --home="${RUNED_HOME}" \
      --keyring-backend="file" \
      --recover
  echo "Keyring prepared!"
}

init_configuration() {
  echo "Initialize configuration..."
  echo "Cleaning up existing configuration..."
  test -f "$CLIENT_CONFIG_FILE" && rm -fv "$CLIENT_CONFIG_FILE"
  test -f "$APP_CONFIG_FILE" && rm -fv "$APP_CONFIG_FILE"
  test -f "$CONFIG_FILE" && rm -fv "$CONFIG_FILE"

  echo "${KEYRING_MNEMONIC}" | runed \
    init \
    "${RUNED_MONIKER}" \
    --chain-id="${RUNED_CHAIN_ID}" \
    --home="${RUNED_HOME}" \
    --keyring-backend="file" \
    --overwrite \
    --recover
  echo "Configuration initialized!"
}

customize_configuration() {
  echo "Backup original configuration..."
  test -f "${CLIENT_CONFIG_FILE}.bak" || cat "$CLIENT_CONFIG_FILE" > "${CLIENT_CONFIG_FILE}.bak"
  test -f "${APP_CONFIG_FILE}.bak" || cat "$APP_CONFIG_FILE" > "${APP_CONFIG_FILE}.bak"
  test -f "${CONFIG_FILE}.bak" || cat "$CONFIG_FILE" > "${CONFIG_FILE}.bak"

  echo "Set configuration defaults..."

  #
  # FILE: client.toml
  #
  runed toml set "$CLIENT_CONFIG_FILE" \
    --home="${RUNED_HOME}" \
    -v "chain-id=${RUNED_CHAIN_ID}" \
    -v "keyring-backend=file"

  # Check if RUNED_CUSTOM_CONF_CLIENT_TOML file exist
  if [ -f "$RUNED_CUSTOM_CONF_CLIENT_TOML" ]; then
    echo "External customizations for client.toml..."
    while IFS= read -r line; do
      runed toml set $CLIENT_CONFIG_FILE --home="${RUNED_HOME}" -v $line
    done < "$RUNED_CUSTOM_CONF_CLIENT_TOML"
  fi

  #
  # FILE: config.toml
  #
  runed toml set "$CONFIG_FILE" \
    --home="${RUNED_HOME}" \
    -v "moniker=${RUNED_MONIKER}" \
    -v "p2p.laddr=tcp://0.0.0.0:${RUNED_PORT_P2P}" \
    -v "p2p.external_address=${PUBLIC_IP}:${RUNED_PORT_P2P}" \
    -v "rpc.laddr=tcp://0.0.0.0:26657" \
    -v "instrumentation.prometheus=true" \
    -v "instrumentation.prometheus_listen_addr=0.0.0.0:26660"

  # Check if RUNED_CUSTOM_CONF_CONFIG_TOML file exist
  if [ -f "$RUNED_CUSTOM_CONF_CONFIG_TOML" ]; then
    echo "External customizations for config.toml..."
    while IFS= read -r line; do
      runed toml set $CONFIG_FILE --home="${RUNED_HOME}" -v $line
    done < "$RUNED_CUSTOM_CONF_CONFIG_TOML"
  fi

  #
  # FILE: app.toml
  #
  runed toml set "$APP_CONFIG_FILE" \
    --home="${RUNED_HOME}" \
    -v "oracle.oracle_address=${RUNED_ORACLE_ORACLE_ADDRESS:-connect-sidecar:8080}" \
    -v "oracle.enabled=true" \
    -v "api.enable=true" \
    -v "api.address=tcp://0.0.0.0:1317" \
    -v "grpc.enable=true" \
    -v "grpc.address=0.0.0.0:9090" \
    -v "grpc-web.enable=true" \
    -v "json-rpc.enable=true" \
    -v "json-rpc.address=0.0.0.0:8545" \
    -v "json-rpc.api=eth,txpool,personal,net,debug,web3,mezo" \
    -v "json-rpc.ws-address=0.0.0.0:8546" \
    -v "json-rpc.metrics-address=0.0.0.0:6065" \
    -v "pruning=nothing"

  # Check if RUNED_CUSTOM_CONF_APP_TOML file exist
  if [ -f "$RUNED_CUSTOM_CONF_APP_TOML" ]; then
    echo "External customizations for app.toml..."
    while IFS= read -r line; do
      runed toml set $APP_CONFIG_FILE --home="${RUNED_HOME}" -v $line
    done < "$RUNED_CUSTOM_CONF_APP_TOML"
  fi

  echo "Configuration customized!"
}

init_genval() {
  test -f "${RUNED_HOME}"/config/genval/genval-*.json && {
    echo "Genval already exists!"
    return
  }

  echo "Prepare genval..."
  echo "${KEYRING_PASSWORD}" \
    | runed genesis genval \
      "${KEYRING_NAME}" \
      --keyring-backend="file" \
      --chain-id="${RUNED_CHAIN_ID}" \
      --home="${RUNED_HOME}" \
      --ip="${PUBLIC_IP}"

  echo "Genval prepared!"
}

get_validator_info() {
  validator_addr_bech="$(echo "${KEYRING_PASSWORD}" | runed --home="${RUNED_HOME}" keys show "${KEYRING_NAME}" --keyring-backend=file --address)"
  validator_addr="$(runed --home="${RUNED_HOME}" keys parse "${validator_addr_bech}" | grep bytes | awk '{print "0x"$2}')"
  echo "Validator address: ${validator_addr}"

  validator_id="$(cat "${RUNED_HOME}"/config/genval/genval-*.json | jq -r '.memo' | awk -F'@' '{print $1}')"
  echo "Validator ID: ${validator_id}"

  validator_consensus_pubkey_bech="$(cat "${RUNED_HOME}"/config/genval/genval-*.json | jq -r '.validator.cons_pub_key_bech32')"
  validator_consensus_pubkey="$(runed --home="${RUNED_HOME}" keys parse "${validator_consensus_pubkey_bech}" | grep bytes | awk '{printf "%s", $2}' | tail -c 64 | awk '{print "0x"$1}')"
  echo "Validator consensus pubkey: ${validator_consensus_pubkey}"

  validator_consensus_addr="$(jq -r '.address' "${RUNED_HOME}"/config/priv_validator_key.json | awk '{print "0x"$1}')"
  echo "Validator consensus address: ${validator_consensus_addr}"

  echo "Moniker: $RUNED_MONIKER"
}

#
# MAIN
#
if [ -z "$1" ]; then
  echo "No command provided!"
  exit 1
fi

case "$1" in
  keyring)
    init_keyring
    exit 0
    ;;
  genval)
    init_genval
    exit 0
    ;;
  info)
    get_validator_info
    exit 0
    ;;
  config)
    init_configuration
    customize_configuration
    exit 0
    ;;
  *)
    init_keyring
    init_configuration
    customize_configuration
    init_genval
    get_validator_info
    # Run the runed node
    exec "$@"
    ;;
esac
