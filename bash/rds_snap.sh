#!/bin/bash


# Get account ID
account_id=$(aws sts get-caller-identity | jq -r '.Account')

## Get binary
aws s3 cp s3://< go-binary-location >/rdssnap .

## Set +x on snapshotizer
chmod +x ./rdssnap

## run binary
./rdssnap arn:aws-us-gov:kms:us-gov-west-1:<acctnum>:key/mrk-KEYNUM <acctnum>

## Optional, copy the results to a central location for record keeping
## backup result to central bucket
aws s3 cp ./instance_snapshots.txt s3://backup-ssm-<acctnum>/rds/instance_snapshots_$account_id.txt

## backup cluster result to central bucket
aws s3 cp ./cluster_snapshots.txt s3://backup-ssm-<acctnum>/rds/cluster_snapshots_$account_id.txt
