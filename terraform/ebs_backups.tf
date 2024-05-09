
resource "aws_kms_key" "ebs_backup" {
  description              = "remote use ebs key"
  deletion_window_in_days  = 10
  key_usage                = "ENCRYPT_DECRYPT"
  is_enabled               = true
  enable_key_rotation      = true
  multi_region             = true
  customer_master_key_spec = "SYMMETRIC_DEFAULT"
  policy                   = data.aws_iam_policy_document.ebs_backup_key_policy.json
  tags = {
    "Name" = "ebs_backup remote key",
  }
}

# Key must allow the org to encrypt/decrypt data with the key, in order to access files in the s3 bucket
data "aws_iam_policy_document" "ebs_backup_key_policy" {
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
      "kms:*"
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


resource "aws_kms_key" "ebs_backup_local" {
  description              = "local use ebs key"
  deletion_window_in_days  = 10
  key_usage                = "ENCRYPT_DECRYPT"
  is_enabled               = true
  enable_key_rotation      = true
  multi_region             = true
  customer_master_key_spec = "SYMMETRIC_DEFAULT"
  policy                   = data.aws_iam_policy_document.ebs_backup_key_policy_local.json
  tags = {
    "Name" = "ebs_backup key for local use",
  }
}

# Key must allow the org to encrypt/decrypt data with the key, in order to access files in the s3 bucket
data "aws_iam_policy_document" "ebs_backup_key_policy_local" {
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

  #   statement {
  #     effect = "Allow"
  #     principals {
  #       type        = "*"
  #       identifiers = ["*"]
  #     }
  #     actions = [
  #       "kms:*"
  #     ]
  #     resources = ["*"]
  #     condition {
  #       test     = "StringEquals"
  #       variable = "aws:PrincipalOrgID"
  #       values = [
  #         local.distro_org_new
  #       ]
  #     }
  #   }
}