#!/bin/bash
# This script uses an ssm parameter to prevent repeat runs when running from a pipeline
# it depends on premade buckets with bucket policy allowing pushing to the bucket from the remote account
# The terraform for this bucket is included in the /terraform/ folder

# Get account ID
account_id=$(aws sts get-caller-identity | jq -r '.Account')

# Get the parameter indicating whether the backup has been run already
s3_backup_run=$(aws ssm get-parameter --name "s3_backup_run" --query "Parameter.Value" --output text 2>/dev/null)

# Get a list of all S3 buckets in the current account
BUCKETS=$(aws s3api list-buckets --query "Buckets[].Name" --output text)

# Output file
OUTPUT_FILE="s3backup-${account_id}-results.txt"

# Loop over each bucket and sync it to the target account
if [ "$s3_backup_run" != "ran" ] ; then
  for BUCKET in $BUCKETS
  do
      # Target bucket name
      TARGET_BUCKET="s3backup-${account_id}/${BUCKET}"

      # Sync the source bucket to the target bucket
      if ! aws s3 sync s3://${BUCKET} s3://${TARGET_BUCKET} >> ${OUTPUT_FILE}; then
        echo "Sync operation failed for bucket: ${BUCKET}"
        exit 1
      fi
  done
  if aws s3 cp ${OUTPUT_FILE} s3://<central bucket>/s3/s3_backup_results_$account_id.txt; then
    aws ssm put-parameter --name "s3_backup_run" --type "String" --overwrite --value "ran"
  else
    echo "Failed to copy the output file to the S3 bucket"
    exit 1
  fi
fi