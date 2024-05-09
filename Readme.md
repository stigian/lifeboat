# Lifeboat

This code was written to perform mass-exfiltration of EBS and RDS snapshots in a DR scenario.

## Usage

```
snapshotizer <destination_KMS_Key_Arn> <account_id_to_share_to>

receivesnaps <destination_KMS_Key_Arn> <snapshot_id_file>

rdssnap <destination_KMS_Key_Arn> <account_id_to_share_to>

rdsreceive  <destination_KMS_Key_Arn> <instance_snapshot_ids_file> <cluster_snapshot_id_file>
```