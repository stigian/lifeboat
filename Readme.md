
# Lifeboat

This code was written to perform mass-exfiltration of KMS-encrypted EBS volumes and RDS databases out of multiple accounts in a DR scenario.

## Usage

```
snapshotizer <destination_KMS_Key_Arn> <account_id_to_share_to>

receivesnaps <destination_KMS_Key_Arn> <snapshot_id_file>

rdssnap <destination_KMS_Key_Arn> <account_id_to_share_to>

rdsreceive  <destination_KMS_Key_Arn> <instance_snapshot_ids_file> <cluster_snapshot_id_file>
```

This code is provided AS IS. Use at your own risk. Stigian is not responsible for the use of this code, any data loss that may result, etc.

## Files

snapshot_ids.txt, the input for receivesnaps, is a simple line-oriented text list of aws ebs snapshot ids that is iterated through in order to
ingest ebs snapshots.

instance_snapshots.txt is a list of shared RDS snapshot IDs (in the form of `arn:aws-us-gov:rds:us-gov-west-1:000000000000:snapshot:shared-snapshot-name`), which is iterated through line-by-line to ingest rds snapshots.

cluster_snapshots.txt is a list of shared RDS Cluster Snapshot IDs (in the form of `arn:aws-us-gov:rds:us-gov-west-1:000000000000:cluster-snapshot:shared-snapshot-name`), which is iterate through line-by-line to ingest rds cluster snapshots.