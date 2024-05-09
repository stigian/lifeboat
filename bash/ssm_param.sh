#!/bin/bash
# Get account ID
account_id=$(aws sts get-caller-identity | jq -r '.Account')

# Get Parameter List
param_list=$(aws ssm get-parameters-by-path --path "/" --recursive --query "Parameters[*].Name" --output text)

for param in ${param_list}; do
    echo "Reading $param"
    aws ssm get-parameter --name "$param" --with-decryption >> ./ssm_backup_$account_id.txt
done

aws s3 cp ./ssm_backup_$account_id.txt s3://< central bucket >/ssm_params/ssm_backup_$account_id.txt