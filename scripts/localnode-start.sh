#!/bin/bash

KEYS[0]="dev0"
KEYS[1]="dev1"
KEYS[2]="dev2"
CHAINID="rune_6591-10"
MONIKER="localnode"
# Remember to change to other types of keyring like 'file' in-case exposing to outside world,
# otherwise your balance will be wiped quickly
# The keyring test does not require private key to steal tokens from you
KEYRING="test"
KEYALGO="eth_secp256k1"
LOGLEVEL="info"
# Set dedicated home directory for the runed instance
HOMEDIR="./.localnode"
# to trace evm
#TRACE="--trace"
TRACE=""

# Path variables
CONFIG=$HOMEDIR/config/config.toml
APP_TOML=$HOMEDIR/config/app.toml
GENESIS=$HOMEDIR/config/genesis.json
TMP_GENESIS=$HOMEDIR/config/tmp_genesis.json

# validate dependencies are installed
command -v jq >/dev/null 2>&1 || {
	echo >&2 "jq not installed. More info: https://stedolan.github.io/jq/download/"
	exit 1
}

# used to exit on first error (any non-zero exit code)
set -e

# Reinstall daemon
make install

# User prompt if an existing local node configuration is found.
if [ -d "$HOMEDIR" ]; then
	printf "\nAn existing folder at '%s' was found. You can choose to delete this folder and start a new local node with new keys from genesis. When declined, the existing local node is started. \n" "$HOMEDIR"
	echo "Overwrite the existing configuration and start a new local node? [y/n]"
	read -r overwrite
else
	overwrite="Y"
fi


# Setup local node if overwrite is set to Yes, otherwise skip setup
if [[ $overwrite == "y" || $overwrite == "Y" ]]; then
	# Remove the previous folder
	rm -rf "$HOMEDIR"

	# Set client config
	runed config set client chain-id $CHAINID --home "$HOMEDIR"
	runed config set client keyring-backend $KEYRING --home "$HOMEDIR"

	# If keys exist they should be deleted
	for KEY in "${KEYS[@]}"; do
		KEYS_ADD_OUT=$(runed keys add "$KEY" --keyring-backend $KEYRING --key-type $KEYALGO --home "$HOMEDIR" --output=json)
		MNEMONIC=$(echo $KEYS_ADD_OUT | jq -r '.mnemonic')
		echo '{"secret":"'$MNEMONIC'"}' > $HOMEDIR/"$KEY"_key_seed.json
	done

	# Set moniker and chain-id for Runed (Moniker can be anything, chain-id must be an integer)
	runed init $MONIKER -o --chain-id $CHAINID --home "$HOMEDIR" --ignore-predefined

	# Change parameter token denominations to arune
	jq '.app_state["crisis"]["constant_fee"]["denom"]="arune"' "$GENESIS" >"$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"
	jq '.app_state["evm"]["params"]["evm_denom"]="arune"' "$GENESIS" >"$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"

	if [[ $1 == "pending" ]]; then
		if [[ "$OSTYPE" == "darwin"* ]]; then
			sed -i '' 's/timeout_propose = "3s"/timeout_propose = "30s"/g' "$CONFIG"
			sed -i '' 's/timeout_propose_delta = "500ms"/timeout_propose_delta = "5s"/g' "$CONFIG"
			sed -i '' 's/timeout_prevote = "1s"/timeout_prevote = "10s"/g' "$CONFIG"
			sed -i '' 's/timeout_prevote_delta = "500ms"/timeout_prevote_delta = "5s"/g' "$CONFIG"
			sed -i '' 's/timeout_precommit = "1s"/timeout_precommit = "10s"/g' "$CONFIG"
			sed -i '' 's/timeout_precommit_delta = "500ms"/timeout_precommit_delta = "5s"/g' "$CONFIG"
			sed -i '' 's/timeout_commit = "5s"/timeout_commit = "150s"/g' "$CONFIG"
			sed -i '' 's/timeout_broadcast_tx_commit = "10s"/timeout_broadcast_tx_commit = "150s"/g' "$CONFIG"
		else
			sed -i 's/timeout_propose = "3s"/timeout_propose = "30s"/g' "$CONFIG"
			sed -i 's/timeout_propose_delta = "500ms"/timeout_propose_delta = "5s"/g' "$CONFIG"
			sed -i 's/timeout_prevote = "1s"/timeout_prevote = "10s"/g' "$CONFIG"
			sed -i 's/timeout_prevote_delta = "500ms"/timeout_prevote_delta = "5s"/g' "$CONFIG"
			sed -i 's/timeout_precommit = "1s"/timeout_precommit = "10s"/g' "$CONFIG"
			sed -i 's/timeout_precommit_delta = "500ms"/timeout_precommit_delta = "5s"/g' "$CONFIG"
			sed -i 's/timeout_commit = "5s"/timeout_commit = "150s"/g' "$CONFIG"
			sed -i 's/timeout_broadcast_tx_commit = "10s"/timeout_broadcast_tx_commit = "150s"/g' "$CONFIG"
		fi
	fi

  # enable prometheus metrics
  if [[ "$OSTYPE" == "darwin"* ]]; then
      sed -i '' 's/prometheus = false/prometheus = true/' "$CONFIG"
      sed -i '' 's/prometheus-retention-time = 0/prometheus-retention-time  = 1000000000000/g' "$APP_TOML"
      sed -i '' 's/enabled = false/enabled = true/g' "$APP_TOML"
  else
      sed -i 's/prometheus = false/prometheus = true/' "$CONFIG"
      sed -i 's/prometheus-retention-time  = "0"/prometheus-retention-time  = "1000000000000"/g' "$APP_TOML"
      sed -i 's/enabled = false/enabled = true/g' "$APP_TOML"
  fi

	# set custom pruning settings
	sed -i.bak 's/pruning = "default"/pruning = "custom"/g' "$APP_TOML"
	sed -i.bak 's/pruning-keep-recent = "0"/pruning-keep-recent = "2"/g' "$APP_TOML"
	sed -i.bak 's/pruning-interval = "0"/pruning-interval = "10"/g' "$APP_TOML"

	# Allocate genesis accounts (cosmos formatted addresses)
	for KEY in "${KEYS[@]}"; do
		runed genesis add-account "$KEY" 100000000000000000000000000arune,100000000000000000000000000ahoodi --keyring-backend $KEYRING --home "$HOMEDIR"
	done

	# bc is required to add these big numbers
	total_supply=$(echo "${#KEYS[@]} * 100000000000000000000000000" | bc)
	jq -r --arg total_supply "$total_supply" '.app_state["bank"]["supply"][0]["amount"]=$total_supply' "$GENESIS" >"$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"

	max_gas=10000000 # 10m
	jq -r --arg max_gas "$max_gas" '.consensus["params"]["block"]["max_gas"]=$max_gas' "$GENESIS" >"$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"

	# Create the validator using the standard Cosmos staking flow
	# (replaces the removed PoA `genesis genval`/`collect-genvals` commands).
	KEYRING_PASSWORD=${KEYRING_PASSWORD:-""}
	VAL_KEY="${KEYS[0]}"
	VAL_PUBKEY=$(runed tendermint show-validator --home "$HOMEDIR")

	# Self-delegate a bond so the validator has consensus power. Uses the
	# arune amount allocated to this account via genesis add-account above.
	yes "$KEYRING_PASSWORD" | runed genesis gentx "$VAL_KEY" \
		1000000000000000000000arune \
		--pubkey "$VAL_PUBKEY" \
		--chain-id "$CHAINID" \
		--keyring-backend "$KEYRING" \
		--home "$HOMEDIR"

	# Aggregate all gentx files into the genesis file.
	runed genesis collect-gentxs --home "$HOMEDIR"

	# Run this to ensure everything worked and that the genesis file is setup correctly
	runed genesis validate --home "$HOMEDIR"

	if [[ $1 == "pending" ]]; then
		echo "pending mode is on, please wait for the first block committed."
	fi
fi

# Start the node (remove the --pruning=nothing flag if historical queries are not needed)
runed start --metrics "$TRACE" --log_level $LOGLEVEL --min-gas-prices=0.0001arune --json-rpc.api eth,txpool,personal,net,debug,web3,rune --api.enable --enable-testbed-precompile --home "$HOMEDIR"
