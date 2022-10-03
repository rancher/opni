package v2_test

//var _ = Describe("Using the self updating controlv1 service", Ordered, Label(test.Unit, test.Slow), func() {
//	var dirRoot string
//	ctx := context.Background()
//	BeforeAll(func() {
//		projRoot, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
//		Expect(err).To(BeNil())
//		dirRoot = strings.TrimSpace(string(projRoot)) + "/"
//	})
//
//	When("we want to patch binaries", func() {
//		It("Should be able to patch arbitrary bytes together", func() {
//			bytesOld := []byte("hello john")
//			bytesNew := []byte("hello  garfield")
//			patch, err := shared.GeneratePatch(bytesOld, bytesNew)
//			Expect(err).To(BeNil())
//
//			generatedFromOld, err := shared.ApplyPatch(bytesOld, patch)
//			Expect(err).To(BeNil())
//			Expect(generatedFromOld).To(Equal(bytesNew))
//		})
//
//		It("Should get the plugin manifests and fetch their compressed data", func() {
//			// get plugin manifests
//			shared.PluginPathTemplate = template.Must(
//				template.New("").Parse(dirRoot + "bin/plugins/plugin_{{.MatchExpr}}"))
//			a := &v2.Agent{
//				Logger: logger.NewPluginLogger().Named("agentv2"),
//			}
//			res, err := a.GetPluginManifests(ctx, &emptypb.Empty{})
//			Expect(err).To(Succeed())
//			Expect(res.Items).NotTo(BeEmpty())
//
//			for pluginPath, el := range res.Items {
//				Expect(shared.PluginPathRegex().MatchString(pluginPath)).To(BeTrue())
//				Expect(el.Metadata).NotTo(Equal(""))
//				Expect(el.Metadata).NotTo(Equal(shared.UnknownRevision))
//			}
//
//			// get plugin data
//
//			data, err := a.GetCompressedManifests(ctx, res)
//			Expect(err).To(Succeed())
//			Expect(len(data.Items)).To(Equal(len(res.Items)))
//			for pluginPath, d := range data.Items {
//				ff, err := os.Stat(pluginPath)
//				Expect(err).To(Succeed())
//
//				Expect(d.Data).NotTo(BeEmpty())
//				Expect(len(d.Data)).To(Equal(int(ff.Size())))
//			}
//		})
//
//		It("Should calculate the operations on binaries that it needs to patch", func() {
//			plugin1UUID := uuid.New().String()
//			plugin2UUID := uuid.New().String()
//			plugin1MetadataUUID := uuid.New().String()
//			plugin2MetadataUUIDAgent := uuid.New().String()
//			plugin2MetadataUUIDGateway := uuid.New().String()
//			plugin3UUID := uuid.New().String()
//			plugin3MetadataUUID := uuid.New().String()
//			plugin4UUID := uuid.New().String()
//			plugin4MetadataUUID := uuid.New().String()
//
//			agentList := &controlv1.ManifestMetadataList{
//				Items: map[string]*controlv1.ManifestMetadata{
//					plugin1UUID: {
//						Metadata: plugin1MetadataUUID,
//					},
//					plugin2UUID: {
//						Metadata: plugin2MetadataUUIDAgent,
//					},
//					plugin3UUID: {
//						Metadata: plugin3MetadataUUID,
//					},
//				},
//			}
//
//			gatewayList := &controlv1.ManifestMetadataList{
//				Items: map[string]*controlv1.ManifestMetadata{
//					plugin1UUID: {
//						Metadata: plugin1MetadataUUID,
//					},
//					plugin2UUID: {
//						Metadata: plugin2MetadataUUIDGateway,
//					},
//					plugin4UUID: {
//						Metadata: plugin4MetadataUUID,
//					},
//				},
//			}
//
//			patchOps, err := gatewayList.LeftJoinOn(agentList)
//			Expect(err).To(Succeed())
//			Expect(patchOps.Items).NotTo(BeEmpty())
//			if _, ok := patchOps.Items[plugin1UUID]; ok {
//				Fail("identical plugins should not be in incoming operations")
//			}
//			Expect(patchOps.Items[plugin4UUID].Op).To(Equal(controlv1.PatchOp_CREATE))
//			Expect(patchOps.Items[plugin3UUID].Op).To(Equal(controlv1.PatchOp_REMOVE))
//			Expect(patchOps.Items[plugin2UUID].Op).To(Equal(controlv1.PatchOp_UPDATE))
//		})
//
//		It("Should be able to patch one set of binaries onto another", func() {
//			goBinaryTemplate := template.Must(template.New("").Parse(`
//package main
//
//import (
//	"fmt"
//)
//
//func main() {
//	fmt.Println("{{.Phrase1}}")
//    fmt.Println("{{.Phrase2}}")
//}
//`))
//			a := &v2.Agent{
//				Logger: logger.NewPluginLogger().Named("agentv2"),
//			}
//			gatewayPluginDir := "/tmp/testbin/plugins/gateway/"
//			err := os.RemoveAll(gatewayPluginDir)
//			Expect(err).To(Succeed())
//			err = os.MkdirAll(gatewayPluginDir, 0755)
//			Expect(err).To(Succeed())
//			agentPluginDir := "/tmp/testbin/plugins/agent/"
//			err = os.RemoveAll(agentPluginDir)
//			Expect(err).To(Succeed())
//			err = os.MkdirAll(agentPluginDir, 0755)
//			Expect(err).To(Succeed())
//
//			createBinaryCode := func(pluginBackend, binaryName, phrase1, phrase2 string) error {
//				var b bytes.Buffer
//				err := goBinaryTemplate.Execute(&b, map[string]string{
//					"Phrase1": phrase1,
//					"Phrase2": phrase2,
//				})
//				Expect(err).To(Succeed())
//				if pluginBackend == "gateway" {
//					err := os.MkdirAll(gatewayPluginDir, 0755)
//					Expect(err).To(Succeed())
//					err = os.WriteFile(gatewayPluginDir+binaryName+".go", b.Bytes(), 0755)
//					Expect(err).To(Succeed())
//					return nil
//				}
//				if pluginBackend == "agent" {
//					err := os.MkdirAll(agentPluginDir, 0755)
//					Expect(err).To(Succeed())
//					err = os.WriteFile(agentPluginDir+binaryName+".go", b.Bytes(), 0755)
//					Expect(err).To(Succeed())
//					return nil
//				}
//				panic("unsupported plugin backend in the test function")
//			}
//
//			build := func(pluginBackend string, name string) error {
//				//preCmd := exec.Command(
//				//	"go",
//				//	"mod",
//				//	"init",
//				//	fmt.Sprintf("/tmp/testbin/plugins/%s", pluginBackend))
//				//preCmd.Stdout = os.Stdout
//				//preCmd.Stderr = os.Stderr
//				//err := preCmd.Run()
//				//if err != nil {
//				//	return err
//				//}
//				cmd := exec.Command("go",
//					"build",
//					"-o", fmt.Sprintf("/tmp/testbin/plugins/%s/%s", pluginBackend, name),
//					fmt.Sprintf("/tmp/testbin/plugins/%s/%s.go", pluginBackend, name),
//				)
//				cmd.Stdout = os.Stdout
//				cmd.Stderr = os.Stderr
//				err = cmd.Run()
//				if err != nil {
//					return err
//				}
//				return nil
//			}
//
//			outdatedPluginName := "plugin_1"
//			newestPluginName := "plugin_1"
//			removePluginName := "plugin_3"
//			createPluginName := "plugin_4"
//
//			err = createBinaryCode("gateway", newestPluginName, "goodbye", "jojo")
//			Expect(err).To(Succeed())
//			err = createBinaryCode("agent", outdatedPluginName, "hello", "world")
//			Expect(err).To(Succeed())
//			err = createBinaryCode("agent", removePluginName, "remove", "me")
//			Expect(err).To(Succeed())
//			err = createBinaryCode("gateway", createPluginName, "create", "me")
//			Expect(err).To(Succeed())
//
//			err = build("gateway", newestPluginName)
//			Expect(err).To(Succeed())
//			err = build("agent", outdatedPluginName)
//			Expect(err).To(Succeed())
//			err = build("agent", removePluginName)
//			Expect(err).To(Succeed())
//			err = build("gateway", createPluginName)
//			Expect(err).To(Succeed())
//
//			err = os.Remove(agentPluginDir + outdatedPluginName + ".go")
//			Expect(err).To(Succeed())
//			err = os.Remove(agentPluginDir + removePluginName + ".go")
//			Expect(err).To(Succeed())
//			err = os.Remove(gatewayPluginDir + createPluginName + ".go")
//			Expect(err).To(Succeed())
//			err = os.Remove(gatewayPluginDir + newestPluginName + ".go")
//			Expect(err).To(Succeed())
//
//			agentPathTpl := template.Must(template.New("").Parse(agentPluginDir + "plugin_{{.MatchExpr}}"))
//			gatewayPathTpl := template.Must(template.New("").Parse(gatewayPluginDir + "plugin_{{.MatchExpr}}"))
//
//			shared.PluginPathTemplate = agentPathTpl
//			agentList, err := a.GetPluginManifests(ctx, &emptypb.Empty{})
//			Expect(err).To(Succeed())
//
//			shared.PluginPathTemplate = gatewayPathTpl
//			gatewayList, err := a.GetPluginManifests(ctx, &emptypb.Empty{})
//			Expect(err).To(Succeed())
//
//			updateTODO, err := gatewayList.LeftJoinOn(agentList)
//			Expect(err).To(Succeed())
//			Expect(updateTODO.Items).NotTo(BeEmpty())
//			Expect(updateTODO.Items[removePluginName].Op).To(Equal(controlv1.PatchOp_REMOVE))
//			Expect(updateTODO.Items[createPluginName].Op).To(Equal(controlv1.PatchOp_CREATE))
//Expect(updateTODO.Items[outdatedPluginName].Op).To(Equal(controlv1.PatchOp_UPDATE))

//shared.PluginPathTemplate = agentPathTpl
//
// Need a way to access two different paths : gateway and agent before this test works

//inputToPatchRPC, err := controlv1.GenerateManifestListFromPatchOps(
//	updateTODO,
//	&shared.NoCache{},
//	&shared.NoCompression{},
//	ctx,
//	a,
//)
//Expect(err).To(Succeed())
//failed, err := a.PatchManifests(ctx, inputToPatchRPC)
//Expect(err).To(Succeed())
//Expect(failed.Items).To(BeEmpty())
//
//_, err = os.Stat(path.Join(agentPluginDir, createPluginName))
//Expect(err).To(Succeed())
//_, err = os.Stat(path.Join(agentPluginDir, removePluginName))
//Expect(os.IsNotExist(err)).To(BeTrue())
//})
//})
//})
