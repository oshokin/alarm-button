# PowerShell script to determine next SemVer based on commit messages since the last tag.
# This script implements semantic versioning by analyzing Git commit messages.
# Rules (case-insensitive):
#   - any commit subject starting with "major:" -> MAJOR bump
#   - else any starting with "feat:"            -> MINOR bump  
#   - else any starting with "fix:"             -> PATCH bump
# If no tag exists, current version is 1.0.0.
# When bumping MINOR, reset PATCH to 0; when bumping MAJOR, reset MINOR/PATCH to 0.

# Define script parameters for controlling output format.
Param(
  # Switch to enable GitHub Actions output format instead of standard output.
  [switch]$EmitGitHubOutput
)

# Stop execution on any error to ensure script reliability.
$ErrorActionPreference = 'Stop'

# Function to normalize version strings by removing 'v' prefix if present.
# This ensures consistent version parsing regardless of tag format.
function Normalize-Version($v) {
  if ($null -eq $v) { return '1.0.0' }
  return ($v -replace '^[vV]', '')
}

# Attempt to find the latest Git tag to determine current version.
$foundTag = $true
try {
  # Get the most recent Git tag, suppressing error output.
  $rawTag = git describe --tags --abbrev=0 2>$null
  
  # Check if Git command failed or returned empty result.
  if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($rawTag)) {
    $foundTag = $false
    $rawTag = '1.0.0'  # Default version for first release.
  }
} catch {
  # Handle any exceptions during Git tag retrieval.
  $foundTag = $false
  $rawTag = '1.0.0'
}

# Normalize the current version by removing any 'v' prefix.
$currVer = Normalize-Version $rawTag

# Determine the range of commits to analyze based on tag existence.
if ($foundTag) {
  # If tag exists, analyze commits since that tag.
  $logRange = "$rawTag..HEAD"
} else {
  # If no tag exists, analyze all commits in current branch.
  $logRange = 'HEAD'
}

# Collect all commit subjects in the specified range for analysis.
$subjects = @()
try {
  # Extract commit subjects and trim whitespace for clean processing.
  $subjects = git log --format=%s $logRange | ForEach-Object { $_.Trim() }
} catch {
  # Continue with empty array if Git log fails.
}

# Determine the appropriate version bump based on commit message patterns.
# Check in order of precedence: major > minor > patch.
$bump = 'none'
if ($subjects | Where-Object { $_ -match '^(?i)major:' } | Select-Object -First 1) { 
  $bump = 'major' 
}
elseif ($subjects | Where-Object { $_ -match '^(?i)feat:' } | Select-Object -First 1) { 
  $bump = 'minor' 
}
elseif ($subjects | Where-Object { $_ -match '^(?i)fix:' } | Select-Object -First 1) { 
  $bump = 'patch' 
}

# Parse the current version into major, minor, and patch components.
$parts = ($currVer -split '\.')

# Ensure we have all three version components, padding with zeros if needed.
# This fixes the edge case where tags might be incomplete (e.g., "v2.1").
while ($parts.Length -lt 3) { 
  $parts += '0' 
}

# Convert version components to integers for arithmetic operations.
[int]$major = $parts[0]
[int]$minor = $parts[1] 
[int]$patch = $parts[2]

# Calculate the next version based on the determined bump type.
$nextVer = $currVer
switch ($bump) {
  'major' { 
    # Major bump: increment major, reset minor and patch to 0.
    $major += 1; $minor = 0; $patch = 0; $nextVer = "$major.$minor.$patch" 
  }
  'minor' { 
    # Minor bump: increment minor, reset patch to 0.
    $minor += 1; $patch = 0; $nextVer = "$major.$minor.$patch" 
  }
  'patch' { 
    # Patch bump: increment patch only.
    $patch += 1; $nextVer = "$major.$minor.$patch" 
  }
  default { 
    # No bump needed, keep current version.
  }
}

# Determine if a release should be created based on findings.
$hasRelease = 0
if (-not $foundTag) {
  # First release: always create a release with baseline version.
  $hasRelease = 1
  $bump = 'none'
  $nextVer = $currVer
} elseif ($bump -ne 'none') {
  # Subsequent releases: only if semantic commits were found.
  $hasRelease = 1
}

# Output results in the appropriate format based on script parameters.
if ($EmitGitHubOutput -and $env:GITHUB_OUTPUT) {
  # GitHub Actions output format: append to GITHUB_OUTPUT file.
  Add-Content -Path $env:GITHUB_OUTPUT -Value ("last_tag=$currVer")
  Add-Content -Path $env:GITHUB_OUTPUT -Value ("next_tag=$nextVer")
  Add-Content -Path $env:GITHUB_OUTPUT -Value ("bump=$bump")
  Add-Content -Path $env:GITHUB_OUTPUT -Value ("has_release=$hasRelease")
} else {
  # Standard output format: write to console for manual use.
  Write-Output "LAST_TAG=$currVer"
  Write-Output "NEXT_TAG=$nextVer"  
  Write-Output "BUMP=$bump"
  Write-Output "HAS_RELEASE=$hasRelease"
}

