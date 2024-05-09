## s3 buckets for 
locals {
  acctlist = [
    "acctnum1",
    "acctnum2"
  ]
  tags_s3backup = {
    local-resource-name = "s3backups"
  }
}

# Could be foreach'd also if needed
resource "aws_kms_key" "backup_s3" {
  description              = "Key for Core backup_shared s3 bucket"
  deletion_window_in_days  = 10
  key_usage                = "ENCRYPT_DECRYPT"
  is_enabled               = true
  enable_key_rotation      = true
  multi_region             = true
  customer_master_key_spec = "SYMMETRIC_DEFAULT"
  policy                   = data.aws_iam_policy_document.backup_s3_key_policy.json
  tags = {
    "Name" = "backup_s3 s3 key",
  }
}

# Key must allow the org to encrypt/decrypt data with the key, in order to access files in the s3 bucket
data "aws_iam_policy_document" "backup_s3_key_policy" {
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



resource "aws_s3_bucket" "s3backup" {
  for_each = toset(local.acctlist)
  # Only create the bucket with the real name in prod, everywhere else put the account number on it
  bucket        = "s3backup-${each.key}"
  force_destroy = false
  tags = merge(local.tags_s3backup, {
    Name = "backup_s3"
  })
}

resource "aws_s3_bucket_policy" "s3backup" {
  for_each = toset(local.acctlist)
  bucket   = aws_s3_bucket.s3backup["${each.key}"].id
  policy   = data.aws_iam_policy_document.s3backup["${each.key}"].json
}
# Policy follows org example here - https://docs.aws.amazon.com/AmazonS3/latest/userguide/example-bucket-policies.html?icmpid=docs_amazons3_console
# Shares to the acct
# Requires put/get to get sync to work
data "aws_iam_policy_document" "s3backup" {
  for_each = toset(local.acctlist)
  statement {
    principals {
      type        = "AWS"
      identifiers = ["*"]
    }
    actions = [
      "s3:ListBucket", "s3:PutObject*", "s3:GetObject*"
    ]

    resources = [
      "${aws_s3_bucket.s3backup["${each.key}"].arn}", "${aws_s3_bucket.s3backup["${each.key}"].arn}/*"
    ]
    condition {
      test     = "StringEquals"
      variable = "aws:PrincipalAccount"

      values = [
        "${each.key}"
      ]
    }
  }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "s3backup" {
  for_each = toset(local.acctlist)
  bucket   = aws_s3_bucket.s3backup["${each.key}"].id

  rule {
    apply_server_side_encryption_by_default {
      kms_master_key_id = aws_kms_key.backup_s3.arn
      sse_algorithm     = "aws:kms"
    }
  }
}

resource "aws_s3_bucket_versioning" "s3backup" {
  for_each = toset(local.acctlist)
  bucket   = aws_s3_bucket.s3backup["${each.key}"].id
  versioning_configuration {
    status = "Enabled"
  }
}

# in case we want to lifecycle archive this stuff later
resource "aws_s3_bucket_lifecycle_configuration" "s3backup" {
  for_each = toset(local.acctlist)
  bucket   = aws_s3_bucket.s3backup["${each.key}"].id
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

resource "aws_s3_bucket_public_access_block" "s3backup" {
  for_each = toset(local.acctlist)
  bucket   = aws_s3_bucket.s3backup["${each.key}"].id

  block_public_acls       = true
  ignore_public_acls      = true
  block_public_policy     = true
  restrict_public_buckets = false
}



