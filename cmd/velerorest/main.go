package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"github.com/vmware-tanzu/velero/pkg/client"
	"github.com/vmware-tanzu/velero/pkg/cmd/cli"
	"github.com/vmware-tanzu/velero/pkg/cmd/cli/backup"
	"github.com/vmware-tanzu/velero/pkg/cmd/cli/restore"
	"github.com/vmware-tanzu/velero/pkg/cmd/util/flag"
	"github.com/vmware-tanzu/velero/pkg/label"
	"github.com/vmware-tanzu/velero/pkg/util/encode"
	"io/ioutil"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"net/http"
	"os"
	"time"

	clientset "github.com/vmware-tanzu/velero/pkg/generated/clientset/versioned"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func main() {
	router := gin.Default()
	backup := router.Group("/k8s/backup")
	{
		backup.GET("/", getAllBackups)
		backup.GET("/:name", getBackup)
		backup.POST("/:name", createBackup)
		backup.DELETE("/:name", deleteBackup)
	}

	restore := router.Group("/k8s/restore")
	{
		restore.GET("/:name", getRestore)
		restore.POST("/:name", createRestore)
	}

	_ = router.Run("0.0.0.0:2020")
}

var (
	config client.VeleroConfig
)

func init() {
	// Load the config here so that we can extract features from it.
	var erro error
	config, erro = client.LoadConfig()
	if erro != nil {
		fmt.Fprintf(os.Stderr, "WARNING: Error reading config file: %v\n", erro)
	}
}

func outputEncoded(obj runtime.Object, format string) (string, error) {
	// assume we're printing obj
	toPrint := obj

	if meta.IsListType(obj) {
		list, _ := meta.ExtractList(obj)
		if len(list) == 1 {
			// if obj was a list and there was only 1 item, just print that 1 instead of a list
			toPrint = list[0]
		}
	}

	encoded, err := encode.Encode(toPrint, format)
	if err != nil {
		return "", err
	}
	fmt.Println(string(encoded))
	return string(encoded), nil
}

func getAllBackups(c *gin.Context) {
	var listOptions metav1.ListOptions

	f := client.NewFactory("vrest", config)
	veleroClient, err := f.Client()
	err = checkError(c, err)
	if err != nil {return}
	backups, erro := veleroClient.VeleroV1().Backups(f.Namespace()).List(context.TODO(), listOptions)
	err = checkError(c, erro)
	if err != nil {return}
	jsonString, err := outputEncoded(backups, "json")
	var js interface{}
	if err := json.Unmarshal([]byte(jsonString), &js); err != nil{
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": http.StatusInternalServerError,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status": http.StatusOK,
		"data": js,
	})
	return
}

func getBackup(c *gin.Context) {
	backupName := c.Param("name")
	var (
		//details               bool = true
		//insecureSkipTLSVerify bool = true
	)

	config, err := client.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: Error reading config file: %v\n", err)
	}
	//caCertFile := config.CACertFile()

	f := client.NewFactory("vrest", config)
	veleroClient, err := f.Client()
	err = checkError(c, err)
	if err != nil {return}

	back, err := veleroClient.VeleroV1().Backups(f.Namespace()).Get(context.TODO(), backupName, metav1.GetOptions{})
	if back == nil || err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status": http.StatusNotFound,
			"message": err.Error(),
		})
		return
	}
	//c.JSON(	http.StatusOK, gin.H{
	//	"status": http.StatusOK,
	//	"data": back,
	//})
	///////////////////////////////////////////////
	//deleteRequestListOptions := pkgbackup.NewDeleteBackupRequestListOptions(back.Name, string(back.UID))
	//deleteRequestList, err := veleroClient.VeleroV1().DeleteBackupRequests(f.Namespace()).List(context.TODO(), deleteRequestListOptions)
	//if err != nil {
	//	_, err := fmt.Fprintf(os.Stderr, "error getting DeleteBackupRequests for backup %s: %v\n", back.Name, err)
	//	if err != nil {
	//		fmt.Println("DeleteBackupRequest return error.")
	//		return
	//	}
	//}

	opts := label.NewListOptionsForBackup(back.Name)
	podVolumeBackupList, err := veleroClient.VeleroV1().PodVolumeBackups(f.Namespace()).List(context.TODO(), opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error getting PodVolumeBackups for backup %s: %v\n", back.Name, err)
	}
	//jsonOut := backupDetailJson(back, deleteRequestList.Items, podVolumeBackupList.Items,
	//							details, veleroClient, insecureSkipTLSVerify, caCertFile)

	c.JSON(	http.StatusOK, gin.H{
		"status": http.StatusOK,
		"data": back,
		"podVolume": podVolumeBackupList,
	})

	return
}

type backupDetails struct {
	name string             `json:"name"`
	namespace string        `json:"namespace"`         // which ns this backup job stayed.
	labels map[string]string         `json:"labels"`
	annotations map[string]string    `json:"annotations"`
	phase string            `json:"phase"`
	backupFmtVer uint16     `json:"backup-format-version"`
	errors     int          `json:"errors"`
	warnings   int          `json:"warnings"`
	namespaceInclude []string       `json:"namespace-include"`
	namespaceExclude []string       `json:"namespace-exclude"`
	started string          `json:"started"`
	completed string        `json:"completed"`
	totalItems uint32       `json:"total-items"`
	totalBackupItems uint32 `json:"total-backup-items"`
	resourceList map[string][]string      `json:"resource-list"`
	resticBackupList  map[string]string   `json:"restic-backup-list"`
	backupVolumeList    []velerov1api.PodVolumeBackup          `json:"backup-volume-list"`
}

func backupDetailJson(
	backup *velerov1api.Backup,
	deleteRequests []velerov1api.DeleteBackupRequest,
	podVolumeBackups []velerov1api.PodVolumeBackup,
	details bool,
	veleroClient clientset.Interface,
	insecureSkipTLSVerify bool,
	caCertFile string,
) backupDetails {
	backupD := backupDetails{}
	backupD.name = backup.ObjectMeta.Name
	backupD.namespace = backup.ObjectMeta.Namespace
	backupD.annotations = backup.ObjectMeta.Annotations
	backupD.labels = backup.ObjectMeta.Labels
	backupD.phase = string(backup.Status.Phase)
	backupD.errors = backup.Status.Errors
	backupD.warnings = backup.Status.Warnings
	backupD.namespaceInclude = backup.Spec.IncludedNamespaces
	backupD.namespaceExclude = backup.Spec.ExcludedNamespaces
	//backupD.resourceList =
	backupD.backupVolumeList = podVolumeBackups

	return backupD
}

func createBackup(c *gin.Context) {
	var reqBody map[string]string
	_data, _ := ioutil.ReadAll(c.Request.Body)
	err := json.Unmarshal(_data, &reqBody)
	if err != nil {
		fmt.Printf("convert json to map failed: %v\n", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": http.StatusInternalServerError,
			"message": err.Error(),
		})
		return
	}

	// do annotation to backup PVs first.
	AnnotatePVs(reqBody["namespace"], []string{}, []string{})

	backupName := c.Param("name")
	f := client.NewFactory("vrest", config)
	o := backup.NewCreateOptions()

	// default to backup all PVs, that we don't need to make annotation to them.
	// WARN: this is not work....
	// version 1.5.2 works now.
	o.DefaultVolumesToRestic = flag.OptionalBool{}
	o.DefaultVolumesToRestic.Set("true")

	var args = []string{backupName}
	fmt.Println(backupName, reqBody["namespace"])
	o.IncludeNamespaces = flag.NewStringArray(reqBody["namespace"])
	err = checkError(c, o.Complete(args, f))
	if err != nil {
		return
	}
	err = checkError(c, o.Validate(nil, args, f))
	if err != nil {return}
	_backup, e := o.BuildBackup(f.Namespace())
	if e != nil {
		return
	}
	_, err = o.GetClient().VeleroV1().Backups(_backup.Namespace).Create(context.TODO(), _backup, metav1.CreateOptions{})
	if err != nil {
		fmt.Printf("create backup failed.%v\n", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": http.StatusInternalServerError,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": http.StatusOK,
		"message": "create backup done, query for progress.",
	})
}

func deleteBackup(c *gin.Context) {
	o := cli.NewDeleteOptions("backup")
	backupName := c.Param("name")
	var args = []string{ backupName }

	f := client.NewFactory("vrest", config)
	o.Confirm = true
	err := checkError(c, o.Complete(f, args))
	if err != nil {return}
	err = checkError(c, o.Validate(nil, f, args))
	if err != nil {return}
	err = backup.Run(o)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": http.StatusInternalServerError,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": http.StatusOK,
		"message": "delete backup done, query for progress.",
	})
}

func checkError(c *gin.Context, err error) error {
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": http.StatusInternalServerError,
			"message": err.Error(),
		})
	}
	return err
}

func createRestore(c *gin.Context) {
	o := restore.NewCreateOptions()
	backupName := c.Param("name")
	o.BackupName = backupName
	var restoreName = []string{fmt.Sprintf("%s-%s", backupName, time.Now().Format("20060102150405"))}
	f := client.NewFactory("vrest", config)
	err := checkError(c, o.Complete(restoreName, f))
	if err != nil {return}
	err = checkError(c, o.Validate(nil, restoreName, f))
	if err != nil {return}
	err = checkError(c, o.Run(nil, f))
	if err != nil {
		return
	} else {
		c.JSON(http.StatusOK, gin.H{
			"status": http.StatusOK,
			"message": map[string]string{"restore_name": restoreName[0]},  // return restore name, which is not same with backup name
		})
	}
}

func getRestore(c *gin.Context) {
	backupName := c.Param("name")
	f := client.NewFactory("vrest", config)
	veleroClient, err := f.Client()
	err = checkError(c, err)
	if err != nil {return}
	rest, err := veleroClient.VeleroV1().Restores(f.Namespace()).
		              Get(context.TODO(), backupName, metav1.GetOptions{})
	if err != nil {
		fmt.Printf("get restore task details caught error: %s\n", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": http.StatusInternalServerError,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status": http.StatusOK,
		"data": *rest,
	})
}