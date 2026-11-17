package runtimeassets

import "testing"

// TestDecideRepairCopyMatrix 覆盖 shouldCopy 的完整判定矩阵（F-A1-19）：
// 3 种 RepairMode × 6 种资产 × 缺失/正常/目录/截断/执行失败。
func TestDecideRepairCopyMatrix(t *testing.T) {
	okCore := AssetHealth{Key: AssetCore, Exists: true, Valid: true, Ready: true, VersionProbeOK: true}
	okWintun := AssetHealth{Key: AssetWintun, Exists: true, Valid: true, Ready: true}
	okGeo := AssetHealth{Key: AssetGeoIP, Exists: true, Valid: true, Ready: true}

	cases := []struct {
		name       string
		mode       RepairMode
		key        AssetKey
		health     AssetHealth
		appUpdated bool
		copiedSHA  string
		hasRecord  bool
		wantCopy   bool
		wantReason string
	}{
		// Force 永远重拷
		{"force-core-ok", RepairForce, AssetCore, okCore, false, "", false, true, repairReasonForce},
		{"force-geo-ok", RepairForce, AssetGeoIP, okGeo, false, "", false, true, repairReasonForce},

		// Invalid：一切正常 -> 不拷
		{"invalid-core-ok", RepairInvalid, AssetCore, okCore, false, "", false, false, repairReasonAlreadyOK},
		{"invalid-wintun-ok", RepairInvalid, AssetWintun, okWintun, false, "", false, false, repairReasonAlreadyOK},
		{"invalid-geo-ok", RepairInvalid, AssetGeoIP, okGeo, false, "", false, false, repairReasonAlreadyOK},

		// Invalid：缺失 / 目录 / 截断 / 执行失败 -> 必须拷
		{"invalid-core-missing", RepairInvalid, AssetCore, AssetHealth{ErrorCode: ErrMissing}, false, "", false, true, repairReasonNeedsRepair},
		{"invalid-core-execfailed", RepairInvalid, AssetCore, AssetHealth{Exists: true, Valid: true, Ready: false, ErrorCode: ErrExecFailed}, false, "", false, true, repairReasonNeedsRepair},
		{"invalid-core-isdir", RepairInvalid, AssetCore, AssetHealth{Exists: true, ErrorCode: ErrIsDir}, false, "", false, true, repairReasonNeedsRepair},
		{"invalid-wintun-invalidpe", RepairInvalid, AssetWintun, AssetHealth{Exists: true, ErrorCode: ErrInvalidPE}, false, "", false, true, repairReasonNeedsRepair},
		{"invalid-wintun-isdir", RepairInvalid, AssetWintun, AssetHealth{Exists: true, ErrorCode: ErrIsDir}, false, "", false, true, repairReasonNeedsRepair},
		{"invalid-geo-corrupt", RepairInvalid, AssetGeoIP, AssetHealth{Exists: true, ErrorCode: ErrCorrupt}, false, "", false, true, repairReasonNeedsRepair},
		{"invalid-geo-isdir", RepairInvalid, AssetGeoIP, AssetHealth{Exists: true, ErrorCode: ErrIsDir}, false, "", false, true, repairReasonNeedsRepair},

		// MissingOnly：只补"实际不存在"的（缺失/目录/截断）
		{"missing-core-missing", RepairMissingOnly, AssetCore, AssetHealth{ErrorCode: ErrMissing}, false, "", false, true, repairReasonNeedsRepair},
		{"missing-core-execfailed-skip", RepairMissingOnly, AssetCore, AssetHealth{Exists: true, Valid: true, Ready: false, ErrorCode: ErrExecFailed}, false, "", false, false, repairReasonAlreadyOK},
		{"missing-geo-corrupt", RepairMissingOnly, AssetGeoIP, AssetHealth{Exists: true, ErrorCode: ErrCorrupt}, false, "", false, true, repairReasonNeedsRepair},
		{"missing-geo-ok", RepairMissingOnly, AssetGeoIP, okGeo, false, "", false, false, repairReasonAlreadyOK},

		// 升级覆盖：app 清单变了才用哈希判断
		{"upgrade-hash-equal-copy", RepairInvalid, AssetCore, AssetHealth{Key: AssetCore, Exists: true, Valid: true, Ready: true, VersionProbeOK: true, SHA256: "sha111"}, true, "sha111", true, true, repairReasonUpgradeIntact},
		{"upgrade-hash-diff-skip", RepairInvalid, AssetCore, AssetHealth{Key: AssetCore, Exists: true, Valid: true, Ready: true, VersionProbeOK: true, SHA256: "sha222"}, true, "sha999", true, false, repairReasonUpgradeEdited},
		{"upgrade-no-record-skip", RepairInvalid, AssetCore, okCore, true, "", false, false, repairReasonUpgradeNoRec},
		{"no-upgrade-ok-skip", RepairInvalid, AssetCore, okCore, false, "sha111", true, false, repairReasonAlreadyOK},
	}

	for _, tc := range cases {
		got := decideRepairCopy(tc.mode, tc.key, tc.health, tc.appUpdated, tc.copiedSHA, tc.hasRecord)
		if got.Copy != tc.wantCopy || got.Reason != tc.wantReason {
			t.Errorf("%s: got copy=%v reason=%q, want copy=%v reason=%q",
				tc.name, got.Copy, got.Reason, tc.wantCopy, tc.wantReason)
		}
	}
}

// F-A1-20：Ready 不掺 geo，ReadyForAll 才是三层全就绪；Required 随场景标注。
func TestStatusReadinessAndRequirement(t *testing.T) {
	assets := map[AssetKey]AssetHealth{
		AssetCore:    {Key: AssetCore, Exists: true, Valid: true, Ready: true},
		AssetWintun:  {Key: AssetWintun, Exists: true, Valid: true, Ready: true},
		AssetGeoIP:   {Key: AssetGeoIP, Exists: true, Valid: true, Ready: false, ErrorCode: ErrMissing},
		AssetGeoSite: {Key: AssetGeoSite, Exists: true, Valid: true, Ready: true},
		AssetMMDB:    {Key: AssetMMDB, Exists: true, Valid: true, Ready: true},
		AssetASN:     {Key: AssetASN, Exists: true, Valid: true, Ready: true},
	}

	st := buildStatus(assets)
	if !st.CoreReady || !st.WintunReady {
		t.Fatal("内核/驱动应就绪")
	}
	if !st.Ready {
		t.Fatal("Ready 语义是 core+wintun，应保持 true")
	}
	if st.GeoReady || st.ReadyForAll {
		t.Fatal("geoip 缺失时 GeoReady/ReadyForAll 应为 false")
	}

	annotateRequirement(&st, RequireGeoOnly)
	if len(st.Required) != 4 {
		t.Fatalf("RequireGeoOnly 应标注 4 个规则库资产, 实际 %v", st.Required)
	}
	for _, key := range []AssetKey{AssetGeoIP, AssetGeoSite, AssetMMDB, AssetASN} {
		if !st.Assets[key].Required {
			t.Fatalf("%s 在本场景应标为 Required", key)
		}
	}

	coreOnly := buildStatus(assets)
	annotateRequirement(&coreOnly, RequireCoreOnly)
	if len(coreOnly.Required) != 1 || coreOnly.Required[0] != AssetCore {
		t.Fatalf("RequireCoreOnly 应只标注 core, 实际 %v", coreOnly.Required)
	}
}
