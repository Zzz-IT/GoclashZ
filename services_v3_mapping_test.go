//go:build windows

package main

import (
	"reflect"
	"sort"
	"testing"
)

// TestServicesV3AssignmentMatchesForwarders 校验服务拆分（第二轮 #8）的两个约束：
//
//  1. services_v3_mapping.go 的归属表覆盖全部 108 个绑定方法，且每个方法只出现一次；
//  2. services_v3_generated.go 中三个服务类型的实际方法集合（除生命周期方法外）
//     与归属表逐项一致，即不存在漏生成的转发方法，也不存在未登记的方法。
//
// 说明：ServiceStartup/ServiceShutdown（以及可选的 ServiceName/ServeHTTP）是
// v3 绑定引擎的内部方法，不会进入 bindings，因此不参与 108 的计数。
func TestServicesV3AssignmentMatchesForwarders(t *testing.T) {
	const wantTotal = 108

	if got := len(servicesV3Assignment); got != wantTotal {
		t.Fatalf("servicesV3Assignment 有 %d 个方法，期望 %d", got, wantTotal)
	}
	if got := len(servicesV3ServiceOrder); got != 3 {
		t.Fatalf("servicesV3ServiceOrder 有 %d 项，期望 3 项", got)
	}

	lifecycle := map[string]bool{
		"ServiceName":     true,
		"ServiceStartup":  true,
		"ServiceShutdown": true,
		"ServeHTTP":       true,
	}

	types := map[string]reflect.Type{
		"AppService":    reflect.TypeOf(&AppService{}),
		"CoreService":   reflect.TypeOf(&CoreService{}),
		"UpdateService": reflect.TypeOf(&UpdateService{}),
	}

	expected := map[string][]string{}
	for method, svc := range servicesV3Assignment {
		if _, ok := types[svc]; !ok {
			t.Fatalf("方法 %s 归属到未知服务 %s", method, svc)
		}
		expected[svc] = append(expected[svc], method)
	}

	for svc, typ := range types {
		var actual []string
		for i := 0; i < typ.NumMethod(); i++ {
			name := typ.Method(i).Name
			if lifecycle[name] {
				continue
			}
			actual = append(actual, name)
		}

		sort.Strings(actual)
		want := expected[svc]
		sort.Strings(want)

		if !reflect.DeepEqual(actual, want) {
			t.Errorf("服务 %s 的转发方法集合与归属表不一致：\n实际 %v\n期望 %v", svc, actual, want)
		}
	}

	// 生命周期方法只由 AppService 承接，保证启动/关闭顺序与拆分前一致。
	appSvc := reflect.TypeOf(&AppService{})
	if _, ok := appSvc.MethodByName("ServiceStartup"); !ok {
		t.Error("AppService 必须转发 ServiceStartup")
	}
	if _, ok := appSvc.MethodByName("ServiceShutdown"); !ok {
		t.Error("AppService 必须转发 ServiceShutdown")
	}
	for _, svc := range []string{"CoreService", "UpdateService"} {
		if _, ok := types[svc].MethodByName("ServiceStartup"); ok {
			t.Errorf("%s 不应实现 ServiceStartup（启动由 AppService 承接）", svc)
		}
		if _, ok := types[svc].MethodByName("ServiceShutdown"); ok {
			t.Errorf("%s 不应实现 ServiceShutdown（退出清理由 AppService 承接）", svc)
		}
	}
}
