
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

