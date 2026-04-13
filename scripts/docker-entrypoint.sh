#!/bin/sh
set -e

# If ADMIN_PASSWORD is provided, ensure it's set in the database
if [ -n "$ADMIN_PASSWORD" ]; then
    echo "Setting up admin password from environment variable..."
    # We use 'setup' command. It will fail if password already set, so we ignore error if it exists.
    # But better check if settings.db exists.
    popugate setup "$ADMIN_PASSWORD" --data "$POPUGATE_DATA_DIR" || echo "Setup already completed or failed (likely password already exists)."
fi

# Run the requested command
exec popugate "$@"
