#!/usr/bin/env python3
"""Merge a backup expiry rule with existing S3 lifecycle rules (AWS CLI required)."""
import json
import os
import subprocess

bucket = os.environ['BACKUP_BUCKET']
prefix = os.environ.get('BACKUP_PREFIX', 'clickfix/postgres').rstrip('/') + '/'
days = int(os.environ.get('BACKUP_KEEP_DAYS', '30'))
if days < 1 or not prefix or prefix == '/':
    raise SystemExit('positive retention and a nonempty prefix required')
base = ['aws', '--endpoint-url', os.environ['S3_ENDPOINT'], 's3api']
# Use deployment-provided AWS_* credentials; never echo credentials.
result = subprocess.run(base + ['get-bucket-lifecycle-configuration', '--bucket', bucket], capture_output=True, text=True)
if result.returncode and 'NoSuchLifecycleConfiguration' not in result.stderr:
    raise SystemExit(result.stderr)
rules = json.loads(result.stdout).get('Rules', []) if result.returncode == 0 else []
rule_id = 'clickfix-postgres-retention'
rules = [r for r in rules if r.get('ID') != rule_id]
rules.append({'ID': rule_id, 'Status': 'Enabled', 'Filter': {'Prefix': prefix},
              'Expiration': {'Days': days}, 'NoncurrentVersionExpiration': {'NoncurrentDays': days},
              'AbortIncompleteMultipartUpload': {'DaysAfterInitiation': 1}})
subprocess.run(base + ['put-bucket-lifecycle-configuration', '--bucket', bucket,
                     '--lifecycle-configuration', json.dumps({'Rules': rules})], check=True)
