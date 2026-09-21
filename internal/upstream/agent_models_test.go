package upstream

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// 主 agent 的 gpts.models 可能是空数组。实测抓包（2026-09-21）：用户列表里
// 「鸿蒙开发」是 is_primary_agent=true 且 show_in_ide，但 gpts.models 为 []；
// 模型挂在别的 agent 下。此前只认一个 agent，选到它就当「没有模型」，
// 白白丢掉 agent-center 这整路来源（三路变两路）。
func TestFetchAgentModelsSkipsAgentWithoutModels(t *testing.T) {
	var detailHits []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/agent-center/agents/useragents":
			// 顺序照抓包：鸿蒙开发排第一，它是主 agent 但没有模型。
			_, _ = io.WriteString(w, `{"agents":[`+
				`{"agent_id":"hmos","agent_name":"鸿蒙开发","is_primary_agent":true,"show_in_ide":true,`+
				`"alias":{"alias_zh_cn":"鸿蒙开发"}},`+
				`{"agent_id":"code","agent_name":"CodeAgent","show_in_ide":true,`+
				`"alias":{"alias_zh_cn":"智能体"}},`+
				`{"agent_id":"other","agent_name":"Other","is_primary_agent":true,`+
				`"alias":{"alias_zh_cn":"其他"}}]}`)
		case "/v1/agent-center/agents/detail":
			id := r.URL.Query().Get("agent_id")
			detailHits = append(detailHits, id)
			if r.Header.Get("Agent-Type") != "AgentCenter" {
				t.Errorf("detail 需带 Agent-Type: AgentCenter，got=%q", r.Header.Get("Agent-Type"))
			}
			switch id {
			case "hmos":
				_, _ = io.WriteString(w, `{"agent_name":"鸿蒙开发","gpts":{"models":[]}}`)
			case "code":
				_, _ = io.WriteString(w, `{"agent_name":"CodeAgent","gpts":{"models":[{`+
					`"model_id":"GLM-5.2","model_name":"GLM-5.2",`+
					`"model_parameters":{"context_window":202752,"max_tokens":8192}}]}}`)
			default:
				_, _ = io.WriteString(w, `{"gpts":{"models":[]}}`)
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := New(5 * time.Second)
	c.snapBase = srv.URL
	cred := SignCredential{AccessKeyID: "AK", SecretAccessKey: "SK", SecurityToken: "ST"}

	infos, err := c.fetchAgentModels(cred)
	if err != nil {
		t.Fatalf("主 agent 空模型不应让整路失败: %v（已试 %v）", err, detailHits)
	}
	if len(infos) != 1 || infos[0].ID != "GLM-5.2" {
		t.Fatalf("应回退到有模型的 agent，got=%+v", infos)
	}
	if infos[0].ContextWindow != 202752 || infos[0].MaxTokens != 8192 {
		t.Errorf("参数未解析: %+v", infos[0])
	}
	// CodeAgent 优先于「鸿蒙开发」之外的其他主 agent：它应排在 hmos 之后、
	// other 之前被取到，即 tried 里不应出现 other。
	for _, id := range detailHits {
		if id == "other" {
			t.Fatalf("拿到模型就该停，不应继续试 other；tried=%v", detailHits)
		}
	}
}

// model_name 可能是营销名而非可调用 ID。实测：glm-5.2-sft-harmony 的
// model_name 是 GLM-5.2-ArkTS-SPARK（该 ID 调用返回 002002009.404）。
// 因此必须优先取 model_id。
func TestFetchAgentModelsPrefersModelIDOverMarketingName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/agent-center/agents/useragents":
			_, _ = io.WriteString(w, `{"agents":[{"agent_id":"code","agent_name":"CodeAgent",`+
				`"show_in_ide":true,"alias":{"alias_zh_cn":"智能体"}}]}`)
		case "/v1/agent-center/agents/detail":
			_, _ = io.WriteString(w, `{"gpts":{"models":[{"model_id":"glm-5.2-sft-harmony",`+
				`"model_name":"GLM-5.2-ArkTS-SPARK",`+
				`"model_parameters":{"context_window":131072}}]}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := New(5 * time.Second)
	c.snapBase = srv.URL
	infos, err := c.fetchAgentModels(SignCredential{AccessKeyID: "AK", SecretAccessKey: "SK"})
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 1 {
		t.Fatalf("got=%+v", infos)
	}
	if infos[0].ID != "glm-5.2-sft-harmony" {
		t.Fatalf("应以 model_id 为准，got ID=%q", infos[0].ID)
	}

	// 反过来：只有 model_name 时仍要能取到（不能因为优先 model_id 就丢掉这条）。
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/agent-center/agents/useragents" {
			_, _ = io.WriteString(w, `{"agents":[{"agent_id":"a","is_primary_agent":true}]}`)
			return
		}
		_, _ = io.WriteString(w, `{"gpts":{"models":[{"model_name":"GLM-5.2",`+
			`"model_parameters":{"context_window":202752}}]}}`)
	}))
	defer srv2.Close()

	c2 := New(5 * time.Second)
	c2.snapBase = srv2.URL
	infos2, err := c2.fetchAgentModels(SignCredential{AccessKeyID: "AK", SecretAccessKey: "SK"})
	if err != nil {
		t.Fatal(err)
	}
	if len(infos2) != 1 || infos2[0].ID != "GLM-5.2" {
		t.Fatalf("仅 model_name 时应回退取它，got=%+v", infos2)
	}
}

// 所有 agent 都没模型时必须报错（让 FetchModels 记日志并保留旧目录），
// 不能静默返回空切片假装成功。
func TestFetchAgentModelsErrorsWhenNoAgentHasModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/agent-center/agents/useragents" {
			_, _ = io.WriteString(w, `{"agents":[{"agent_id":"hmos","is_primary_agent":true}]}`)
			return
		}
		_, _ = io.WriteString(w, `{"gpts":{"models":[]}}`)
	}))
	defer srv.Close()

	c := New(5 * time.Second)
	c.snapBase = srv.URL
	if _, err := c.fetchAgentModels(SignCredential{AccessKeyID: "AK", SecretAccessKey: "SK"}); err == nil {
		t.Fatal("全部候选都无模型时应报错，而不是返回空列表")
	}
}
