package api

type AssetRequest struct {
	AssetId        string `json:"asset_id"`
	LocalAssetHash string `json:"asset_hash"`
}

type AssetResponse struct {
	AssetPayload string `json:"asset_payload"`
	CacheHit     bool   `json:"cache_hit"`
}
