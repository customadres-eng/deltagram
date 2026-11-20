param([string]$OutputRoot = "giftpacks-full", [string[]]$OnlySlug)
$ErrorActionPreference = "Stop"
$utf8NoBom = New-Object System.Text.UTF8Encoding($false)
$root = "https://cdn.changes.tg/gifts"
$gifts = @(
  @{ Name = "Astral Shard"; Slug = "astral-shard"; ID = "5933629604416717361" },
  @{ Name = "Durov's Cap"; Slug = "durovs-cap"; ID = "5915521180483191380" },
  @{ Name = "Plush Pepe"; Slug = "plush-pepe"; ID = "5936013938331222567" },
  @{ Name = "Chill Flame"; Slug = "chill-flame"; ID = "5999277561060787166" },
  @{ Name = "Hanging Star"; Slug = "hanging-star"; ID = "5915733223018594841" },
  @{ Name = "Scared Cat"; Slug = "scared-cat"; ID = "5837059369300132790" }
  ,@{ Name = "Kissed Frog"; Slug = "kissed-frog"; ID = "5845776576658015084" }
  ,@{ Name = "Precious Peach"; Slug = "precious-peach"; ID = "5933671725160989227" }
  ,@{ Name = "Loot Bag"; Slug = "loot-bag"; ID = "5868659926187901653" }
  ,@{ Name = "Resistance Dog"; Slug = "resistance-dog"; ID = "5832279504491381684" }
  ,@{ Name = "Desk Calendar"; Slug = "desk-calendar"; ID = "5782988952268964995" }
)
if ($OnlySlug -and $OnlySlug.Count -gt 0) {
  $gifts = @($gifts | Where-Object { $OnlySlug -contains $_.Slug })
}
function Get-Listing([string]$url) {
  $html = (Invoke-WebRequest -Uri $url -UseBasicParsing).Content
  return [regex]::Matches($html, 'href="\./([^"]+\.tgs)"') | ForEach-Object { $_.Groups[1].Value }
}
function Get-Meta([string]$url) { return (Invoke-WebRequest -Uri $url -UseBasicParsing).Content | ConvertFrom-Json }
foreach ($gift in $gifts) {
  $zip = Join-Path $OutputRoot ($gift.Slug + '.zip')
  $existingManifest = Join-Path (Join-Path $OutputRoot $gift.Slug) 'pack.json'
  if ((Test-Path $zip) -and (Test-Path $existingManifest)) {
    Write-Host "$($gift.Name): existing pack preserved"
    continue
  }
  $encoded = [uri]::EscapeDataString($gift.Name)
  $dir = Join-Path $OutputRoot $gift.Slug
  New-Item -ItemType Directory -Force -Path $dir | Out-Null
  $models = Get-Meta "$root/models/$encoded/models.json"
  $patterns = Get-Meta "$root/patterns/$encoded/patterns.json"
  $backdrops = Get-Meta "$root/backdrops/$encoded/backdrops.json"
  $modelMeta = @{}; foreach ($x in $models) { $modelMeta[$x.name] = [int]$x.rarityPermille }
  $patternMeta = @{}; foreach ($x in $patterns) { $patternMeta[$x.name] = [int]$x.rarityPermille }
  Invoke-WebRequest -Uri "$root/originals/$($gift.ID)/Original.tgs" -OutFile (Join-Path $dir 'base.tgs')
  $modelSpecs = @(); foreach ($file in (Get-Listing "$root/models/$encoded/")) {
    $name = [uri]::UnescapeDataString(($file -replace '\.tgs$',''))
    Invoke-WebRequest -Uri "$root/models/$encoded/$file" -OutFile (Join-Path $dir ("model-" + $name + ".tgs"))
    $meta = $models | Where-Object { $_.name -eq $name } | Select-Object -First 1
    $modelSpec = [ordered]@{ name=$name; animation=("model-" + $name + ".tgs"); permille=([int]$modelMeta[$name]) }
    if ($meta.crafted -eq $true) {
      # Crafted attributes are not random-draw entries. The server validator
      # requires a named rarity kind (Telegram uses legendary) with zero
      # permille for crafted models.
      $modelSpec.crafted = $true
      $modelSpec.rarity = 'legendary'
      $modelSpec.permille = 0
    }
    elseif ($modelSpec.permille -lt 1) { $modelSpec.permille = 1 }
    $modelSpecs += $modelSpec
  }
  $patternSpecs = @(); foreach ($file in (Get-Listing "$root/patterns/$encoded/")) {
    $name = [uri]::UnescapeDataString(($file -replace '\.tgs$',''))
    Invoke-WebRequest -Uri "$root/patterns/$encoded/$file" -OutFile (Join-Path $dir ("pattern-" + $name + ".tgs"))
    $patternSpecs += [ordered]@{ name=$name; animation=("pattern-" + $name + ".tgs"); permille=([Math]::Max(1, $patternMeta[$name])) }
  }
  $backdropSpecs = @(); foreach ($b in $backdrops) {
    $backdropSpecs += [ordered]@{ name=$b.name; center=$b.hex.centerColor; edge=$b.hex.edgeColor; pattern=$b.hex.patternColor; text=$b.hex.textColor; permille=[int]$b.rarityPermille }
  }
  $manifest = [ordered]@{ pack_name=$gift.Name; author="Telegram GiftChanges CDN"; gifts=@([ordered]@{
    title=$gift.Name; stars=500; convert_stars=250; base_animation='base.tgs'; upgrade=[ordered]@{
      upgrade_stars=1000; supply_total=10000; slug_prefix=$gift.Slug; models=$modelSpecs; patterns=$patternSpecs; backdrops=$backdropSpecs
    }
  }) }
  $json = $manifest | ConvertTo-Json -Depth 8
  [System.IO.File]::WriteAllText((Join-Path $dir 'pack.json'), $json, $utf8NoBom)
  if (Test-Path $zip) { throw "Refusing to overwrite existing archive: $zip" }
  Compress-Archive -Path (Join-Path $dir '*') -DestinationPath $zip -CompressionLevel Optimal
  Write-Host "$($gift.Name): models=$($modelSpecs.Count) patterns=$($patternSpecs.Count) backdrops=$($backdropSpecs.Count) zip=$((Get-Item $zip).Length)"
}
