#!/bin/sh

set -e

reply=$(curl -s -o /dev/null -w %{http_code} --connect-timeout 3 http://127.0.0.1:9000/minio/health/live)
if [ "$reply" != 200 ]; then
  exit 1;
fi

AWS_ACCESS_KEY_ID=$MINIO_ROOT_USER
AWS_SECRET_ACCESS_KEY=$MINIO_ROOT_PASSWORD
aws s3api list-buckets --endpoint http://localhost:9000 > /dev/null
