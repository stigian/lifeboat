## s3 bucket for ssm parameter text files

## S3 bucket for use in a project
# Used to allow ec2s to output evaluate stig results to central bucket.
# KMS key for bucket encryption
### KMS Key Definition

locals {
  name_backup     = "backup-ssm"
  fullname_backup = "${local.name_backup}-${data.aws_caller_identity.current.account_id}"
  tags_backup = {
    local-resource-name = "${local.name_backup}"
  }
  distro_org = "org-id"

}


resource "aws_kms_key" "ssm_backup" {
  description              = "Key for Core ssm s3 bucket"
  deletion_window_in_days  = 10
  key_usage                = "ENCRYPT_DECRYPT"
  is_enabled               = true
  enable_key_rotation      = true
  multi_region             = true
  customer_master_key_spec = "SYMMETRIC_DEFAULT"
  policy                   = data.aws_iam_policy_document.ssm_backup_key_policy.json
  tags = {
    "Name" = "ssm_backup s3 key",
  }
}

# Key must allow the org to encrypt/decrypt data with the key, in order to access files in the s3 bucket
data "aws_iam_policy_document" "ssm_backup_key_policy" {
  statement {
    effect = "Allow"
    principals {
      type        = "AWS"
      identifiers = ["arn:aws-us-gov:iam::${data.aws_caller_identity.current.account_id}:root"]
    }
    actions = [
      "kms:*"
    ]
    resources = ["*"]
  }

  statement {
    effect = "Allow"
    principals {
      type        = "*"
      identifiers = ["*"]
    }
    actions = [
      "kms:Describe*",
      "kms:List*",
      "kms:Get*",
      "kms:Encrypt",
      "kms:Decrypt",
      "kms:ReEncrypt*",
      "kms:GenerateDataKey"
    ]
    resources = ["*"]
    condition {
      test     = "StringEquals"
      variable = "aws:PrincipalOrgID"
      values = [
        local.distro_org
      ]
    }
  }
}


################################################################################
# The S3 bucket
# with versioning, lifecycle, encryption, logging
resource "aws_s3_bucket" "backup" {
  # Only create the bucket with the real name in prod, everywhere else put the account number on it
  bucket        = local.fullname_backup
  force_destroy = false
  tags = merge(local.tags_backup, {
    Name = "ssm_backup"
  })
}

resource "aws_s3_bucket_policy" "backup" {
  bucket = aws_s3_bucket.backup.id
  policy = data.aws_iam_policy_document.backup.json
}
# Policy follows org example here - https://docs.aws.amazon.com/AmazonS3/latest/userguide/example-bucket-policies.html?icmpid=docs_amazons3_console
# Shares to the ORG
data "aws_iam_policy_document" "backup" {
  statement {
    principals {
      type        = "AWS"
      identifiers = ["*"]
    }

    actions = [
      "s3:PutObject"
    ]

    resources = [
      "${aws_s3_bucket.backup.arn}/*"
    ]
    condition {
      test     = "StringEquals"
      variable = "aws:PrincipalOrgID"

      values = [
        "${local.distro_org}"
      ]
    }
  }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "backup" {
  bucket = aws_s3_bucket.backup.id

  rule {
    apply_server_side_encryption_by_default {
      kms_master_key_id = aws_kms_key.ssm_backup.arn
      sse_algorithm     = "aws:kms"
    }
  }
}

resource "aws_s3_bucket_versioning" "backup" {
  bucket = aws_s3_bucket.backup.id
  versioning_configuration {
    status = "Enabled"
  }
}

# in case we want to lifecycle archive this stuff later
resource "aws_s3_bucket_lifecycle_configuration" "backup" {
  bucket = aws_s3_bucket.backup.id
  rule {
    filter {}
    id = "Expire after 90 days"
    # Enable this after verification
    status = "Disabled"
    noncurrent_version_expiration {
      noncurrent_days = 90
    }
  }
}

resource "aws_s3_bucket_public_access_block" "backup" {
  bucket = aws_s3_bucket.backup.id

  block_public_acls       = true
  ignore_public_acls      = true
  block_public_policy     = true
  restrict_public_buckets = false
}


