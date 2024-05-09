#!/bin/bash
# Get account ID
account_id=$(aws sts get-caller-identity | jq -r '.Account')

## Get binary
aws s3 cp s3://< go binary location >/snapshotizer .

## Set +x on snapshotizer
chmod +x ./snapshotizer

## run binary
./snapshotizer arn:aws-us-gov:kms:us-gov-west-1:<acctnum>:key/mrk-keyid <acctnum>

## Optional reporting to central bucket
## backup result to central bucket
aws s3 cp ./snapshot_ids.txt s3://<centralbucket>/ebs/snapshot_id_$account_id.txt
