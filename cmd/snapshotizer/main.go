package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/sts"
)

func GetInstances(svc *ec2.EC2) ([]*ec2.Reservation, error) {
	instances := []*ec2.Reservation{}
	err := svc.DescribeInstancesPages(
		&ec2.DescribeInstancesInput{},
		func(page *ec2.DescribeInstancesOutput, lastPage bool) bool {
			instances = append(instances, page.Reservations...)
			return !lastPage
		},
	)
	return instances, err
}

func getNameTag(tags []*ec2.Tag) string {
	var name *string

	for _, tag := range tags {
		if aws.StringValue(tag.Key) == "Name" {
			name = tag.Value
		}
	}

	return aws.StringValue(name)
}

func numSnapshotsInProgress(svc *ec2.EC2) (int, error) {
	res, err := svc.DescribeSnapshots(&ec2.DescribeSnapshotsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("status"),
				Values: aws.StringSlice([]string{"pending"}),
			},
		},
	})
	if err != nil {
		return 0, err
	}
	return len(res.Snapshots), nil
}

func snapshotState(svc *ec2.EC2, snapshot string) (string, error) {
	res, err := svc.DescribeSnapshots(&ec2.DescribeSnapshotsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("snapshot-id"),
				Values: aws.StringSlice([]string{snapshot}),
			},
		},
	})
	if err != nil {
		fmt.Println("failed to query state for snapshot ", snapshot, ": ", err)
		return "", err
	}
	return aws.StringValue(res.Snapshots[0].State), err
}

func numInstances(res []*ec2.Reservation) int {
	count := 0
	for _, reservation := range res {
		for _, instance := range reservation.Instances {
			if aws.StringValue(instance.State.Name) != "terminated" {
				count += 1
			}
		}
	}
	return count
}

func main() {

	KMSArn := os.Args[1]
	SharedAccountId := os.Args[2]
	var MAX_SNAPSHOT_COPIES int = 10 //default to 10, override from environment variable
	if sc, err := strconv.Atoi(os.Getenv("MAX_SNAPSHOT_COPIES")); err == nil {
		MAX_SNAPSHOT_COPIES = sc
	}

	sess := session.Must(session.NewSessionWithOptions(
		session.Options{
			SharedConfigState: session.SharedConfigEnable,
		},
	))
	stssvc := sts.New(sess)
	res, err := stssvc.GetCallerIdentity(&sts.GetCallerIdentityInput{})
	if err != nil {
		fmt.Println("failed to get caller identity", err)
		os.Exit(1)
	}
	AccountId := aws.StringValue(res.Account)
	fmt.Println(aws.StringValue(res.Account))

	svc := ec2.New(sess)
	reservations, err := GetInstances(svc)
	if err != nil {
		fmt.Println("failure")
		os.Exit(1)
	}

	OutputSnapshotIds := []string{}
	NumInstances := numInstances(reservations)
	fmt.Println(NumInstances, " Instances detected")
	InstanceProgress := 1
	for _, reservation := range reservations {
		for _, instance := range reservation.Instances {
			if aws.StringValue(instance.State.Name) == "terminated" {
				continue
			}
			fmt.Println("Processing instance", aws.StringValue(instance.InstanceId), "-", InstanceProgress, "of", NumInstances)

			var instanceName string = getNameTag(instance.Tags)
			if instanceName == "" {
				instanceName = aws.StringValue(instance.InstanceId)
			}
			for volumeNum, mapping := range instance.BlockDeviceMappings {
				fmt.Println("Volume", aws.StringValue(mapping.Ebs.VolumeId), "-", volumeNum+1, "of", len(instance.BlockDeviceMappings))
				Snapshot, err := svc.CreateSnapshot(&ec2.CreateSnapshotInput{
					Description: aws.String(fmt.Sprintf("Intermediate snapshot created from volume %d of instance named %s (%s). Delete this.", volumeNum, instanceName, aws.StringValue(instance.InstanceId))),
					VolumeId:    mapping.Ebs.VolumeId,
					TagSpecifications: []*ec2.TagSpecification{
						{
							ResourceType: aws.String("snapshot"),
							Tags: []*ec2.Tag{
								{
									Key:   aws.String("Name"),
									Value: aws.String(fmt.Sprintf("inter-%s-%s-%d", AccountId, instanceName, volumeNum)),
								},
							},
						},
					},
				})
				if err != nil {
					fmt.Printf("failed to create snapshot: %s", err)
				}
				fmt.Println("Intermediate snapshot initiated: ", aws.StringValue(Snapshot.SnapshotId))
				// poll snapshot progress
				state, err := snapshotState(svc, aws.StringValue(Snapshot.SnapshotId))
				if err != nil {
					fmt.Println("failed to query snapshot: ", err)
				}
				fmt.Printf("Intermediate snapshot state: %s ", state)
				for state != "completed" && state != "error" {
					time.Sleep(1 * time.Second)
					state, err = snapshotState(svc, aws.StringValue(Snapshot.SnapshotId))
					if err != nil {
						fmt.Println("Failed to query snapshot state: ", err)
						break
					}
					if state == "pending" {
						fmt.Printf(".")
					} else {
						fmt.Printf("%s\n", state)
					}
				}

				//check that the quota of snapshots in progress hasnt been exceeded
				num, err := numSnapshotsInProgress(svc)
				if err != nil {
					fmt.Println("error querying snapshots in progress: ", err)
				}
				fmt.Println("Background snapshot copies in progress:", num)
				if num >= MAX_SNAPSHOT_COPIES {
					fmt.Printf("Simultaneous snapshot copy limit (%d) exceeded, waiting..", MAX_SNAPSHOT_COPIES)
				}
				for num >= MAX_SNAPSHOT_COPIES {
					fmt.Printf(".")
					time.Sleep(4 * time.Second)
					num, err = numSnapshotsInProgress(svc)
					if err != nil {
						fmt.Println("failed to query snapshot progress: ", err)
						continue
					}
				}
				fmt.Printf("ok\n")

				res, err := svc.CopySnapshot(&ec2.CopySnapshotInput{
					Description: aws.String(fmt.Sprintf(
						"Output Snapshot for account %s instance %s (%s) volume %d",
						AccountId,
						aws.StringValue(instance.InstanceId),
						instanceName,
						volumeNum)),
					KmsKeyId:         aws.String(KMSArn),
					Encrypted:        aws.Bool(true),
					SourceRegion:     aws.String("us-gov-west-1"),
					SourceSnapshotId: Snapshot.SnapshotId,
					TagSpecifications: []*ec2.TagSpecification{
						{
							ResourceType: aws.String("snapshot"),
							Tags: []*ec2.Tag{
								{
									Key:   aws.String("Name"),
									Value: aws.String(fmt.Sprintf("shared-%s-%s-%d", AccountId, instanceName, volumeNum)),
								},
							},
						},
					},
				})
				if err != nil {
					fmt.Println("failed to generate snapshot: ", err)
					os.Exit(1)
				}
				fmt.Printf("Copy initiated using new key: %s -> %s\n", aws.StringValue(Snapshot.SnapshotId), aws.StringValue(res.SnapshotId))
				_, err = svc.ModifySnapshotAttribute(&ec2.ModifySnapshotAttributeInput{
					Attribute:  aws.String("createVolumePermission"),
					SnapshotId: res.SnapshotId,
					CreateVolumePermission: &ec2.CreateVolumePermissionModifications{
						Add: []*ec2.CreateVolumePermission{
							{
								UserId: aws.String(SharedAccountId),
							},
						},
					},
				})
				if err != nil {
					fmt.Println("Failed to share snapshot ", aws.StringValue(res.SnapshotId), ": ", err)
				}
				fmt.Println("Sharing snapshot", aws.StringValue(res.SnapshotId), "to account:", SharedAccountId)
				OutputSnapshotIds = append(OutputSnapshotIds, aws.StringValue(res.SnapshotId))
			}
			InstanceProgress += 1
			fmt.Println()
		}
	}
	f, err := os.Create("snapshot_ids.txt")
	if err != nil {
		fmt.Println("Failed to create file ", err)
	}
	w := bufio.NewWriter(f)
	defer f.Close()

	for _, id := range OutputSnapshotIds {
		fmt.Fprintln(w, id)
		fmt.Println(id)
	}
	w.Flush()
}
