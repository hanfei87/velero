package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

func GetKubeConfigEnv() *string {
	//var kubeconfig *string
	var kubec string
	if home := homeDir(); home != "" {
		//kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
		kubec = filepath.Join(home, ".kube", "config")
	} else {
		//kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
		kubec = ""
	}
	flag.Parse()

	return &kubec
}

// TODO: specific exlude items with map[podName]list[PVname]
func AnnotatePVs(namespace string, excludePv []string, excludePod []string) {
	var kubeconfig *string = GetKubeConfigEnv()

	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err.Error())
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	pods, err := clientset.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		panic(err.Error())
	}
	fmt.Printf("There are %d pods in the cluster\n", len(pods.Items))
	var pvList []string
	for _, p := range pods.Items {
		fmt.Println(p.Name)
		if result, _ := Contain(p.Name, excludePod); result {
			continue
		}
		for _, vol := range p.Spec.Volumes {
			fmt.Println(vol.Name)
			if result, _ := Contain(vol.Name, excludePv); result {
				continue
			}
			if strings.Index(vol.Name, "default-token") == 0 {
				// ignore token pv
				continue
			} else {
				pvList = append(pvList, vol.Name)
			}
		}
		annotationList := strings.Join(pvList, ",")
		annos := p.GetAnnotations()
		if annos == nil {
			annos = make(map[string]string)
		}
		annos["backup.velero.io/backup-volumes"] = annotationList
		p.SetAnnotations(annos)
		clientset.CoreV1().Pods("nginx-example").Update(context.TODO(), &p, metav1.UpdateOptions{})
	}
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}

func Contain(obj interface{}, target interface{}) (bool, error) {
	targetValue := reflect.ValueOf(target)
	switch reflect.TypeOf(target).Kind() {
	case reflect.Slice, reflect.Array:
		for i := 0; i < targetValue.Len(); i++ {
			if targetValue.Index(i).Interface() == obj {
				return true, nil
			}
		}
	case reflect.Map:
		if targetValue.MapIndex(reflect.ValueOf(obj)).IsValid() {
			return true, nil
		}
	}

	return false, errors.New("not in array")
}