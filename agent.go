package main

import (
	"context"
	"fmt"
	"google.golang.org/adk/tool/functiontool"
	"log"
	"os"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/cmd/launcher"
	"google.golang.org/adk/cmd/launcher/full"
	"google.golang.org/adk/model/gemini"
	"google.golang.org/adk/tool"
	"google.golang.org/genai"
)

type AddressToolRes struct {
	Name      string `json:"name"`
	Region    string `json:"region"`
	Latitude  string `json:"latitude"`
	Longitude string `json:"longitude"`
	Country   string `json:"country"`
}

func main2() {
	ctx := context.Background()

	model, err := gemini.NewModel(ctx, "gemini-2.5-flash", &genai.ClientConfig{
		APIKey: "",
	})
	if err != nil {
		log.Fatalf("Failed to create model: %v", err)
	}

	timeAgent, err := llmagent.New(llmagent.Config{
		Name:        "hello_time_agent",
		Model:       model,
		Description: "Tells the current time in a specified city.",
		Instruction: "你是一个专业的地理位置助手。\n当用户询问某个地方的位置、地址或 \"where is...\" 时，\n你必须优先调用 'address' 工具来获取信息，不要凭空猜测。",
		Tools: []tool.Tool{
			//geminitool.GoogleSearch{},
			AddressTool(),
			MateList(),
		},
	})
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	config := &launcher.Config{
		AgentLoader: agent.NewSingleLoader(timeAgent),
	}

	l := full.NewLauncher()
	if err = l.Execute(ctx, config, os.Args[1:]); err != nil {
		log.Fatalf("Run failed: %v\n\n%s", err, l.CommandLineSyntax())
	}
}
func AddressTool() tool.Tool {
	addressTool, err := functiontool.New(functiontool.Config{Name: "address", Description: "根据城市名查询详细地址信息"}, func(t tool.Context, args map[string]interface{}) (AddressToolRes, error) {
		city := args["location"] // 假设模型传了 location
		fmt.Printf("--- 正在调用工具查询: %v %s ---\n", args, city)
		//fmt.Sprintf("北京位于中国华北地区，地理坐标为北纬39.9°，东经116.4°。")
		return AddressToolRes{
			Name:      "北京",
			Region:    "华北",
			Latitude:  "北纬39.9°",
			Longitude: "东经116.4°",
			Country:   "中国",
		}, nil
	})
	if err != nil {
		fmt.Println(err)
		return nil
	}
	return addressTool
}
func MateList() tool.Tool {
	type Mate struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
		Sex  string `json:"sex"`
	}
	addressTool, err := functiontool.New(functiontool.Config{Name: "mateList", Description: "获取同学列表"}, func(t tool.Context, args map[string]interface{}) (map[string]Mate, error) {
		Mates := make(map[string]Mate)
		Mates["小明"] = Mate{
			Name: "小明",
			Age:  11,
			Sex:  "男",
		}
		Mates["小红"] = Mate{
			Name: "小红",
			Age:  14,
			Sex:  "女",
		}
		Mates["小刚"] = Mate{
			Name: "小刚",
			Age:  16,
			Sex:  "男",
		}
		Mates["小美"] = Mate{
			Name: "小美",
			Age:  18,
			Sex:  "女",
		}
		Mates["小强"] = Mate{
			Name: "小强",
			Age:  20,
			Sex:  "男",
		}

		fmt.Printf("--- 正在调用工具查询: %v %s ---\n", args)
		//fmt.Sprintf("北京位于中国华北地区，地理坐标为北纬39.9°，东经116.4°。")
		return Mates, nil
	})
	if err != nil {
		fmt.Println(err)
		return nil
	}
	return addressTool
}
