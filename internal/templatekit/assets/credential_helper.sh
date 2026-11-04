#!/bin/sh
protocol='' host='' path=''
while IFS='=' read -r key value; do
  case "$key" in
    protocol) protocol=$value ;;
    host) host=$value ;;
    path) path=$value ;;
  esac
done
if [ "$1" = get ] && [ "$protocol" = https ] && [ "$host/$path" = "$PMTOOL_TEMPLATE_REPOSITORY.git" ]; then
  printf 'username=%s\npassword=%s\n' "$PMTOOL_TEMPLATE_USERNAME" "$PMTOOL_TEMPLATE_TOKEN"
fi
