package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
)

func getNameTag(tags []*ec2.Tag) string {
	var name *string

	for _, tag := range tags {
		if aws.StringValue(tag.Key) == "Name" {
			name = tag.Value
		}
	}

	return aws.StringValue(name)
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

func main() {
	KMSArn := os.Args[1]
	SnapshotFile := os.Args[2]

	var MAX_SNAPSHOT_COPIES int = 10 //default to 10, override from environment variable
	if sc, err := strconv.Atoi(os.Getenv("MAX_SNAPSHOT_COPIES")); err == nil {
		MAX_SNAPSHOT_COPIES = sc
	}

	f, err := os.Open(SnapshotFile)
	if err != nil {
		fmt.Println("Failed to open snapshots file: ", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	sess := session.Must(session.NewSessionWithOptions(
		session.Options{
			SharedConfigState: session.SharedConfigEnable,
		},
	))
	svc := ec2.New(sess)
	for scanner.Scan() {
		snapshotid := scanner.Text()
		if snapshotid == "" {
			continue
		}
		//get snapshot name and description from input snapshot
		desc, err := svc.DescribeSnapshots(&ec2.DescribeSnapshotsInput{
			Filters: []*ec2.Filter{
				{
					Name:   aws.String("snapshot-id"),
					Values: aws.StringSlice([]string{snapshotid}),
				},
			},
		})
		if err != nil {
			fmt.Println("failed to describe snapshot: ", err)
			continue
		}
		InputSnapshot := desc.Snapshots[0]
		//check if input snapshot is completed
		state, err := snapshotState(svc, snapshotid)
		if err != nil {
			fmt.Println("failed to query snapshot: ", err)
			continue
		}
		fmt.Printf("Status of source snapshot %s: %s", snapshotid, state)
		if state == "completed" || state == "error" {
			fmt.Printf("\n")
		}
		for state != "completed" && state != "error" {
			time.Sleep(1 * time.Second)
			state, err = snapshotState(svc, snapshotid)
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

		//poll on pending snapshot copies to avoid exceeding limits
		num, err := numSnapshotsInProgress(svc)
		if err != nil {
			fmt.Println("error querying snapshots in progress: ", err)
		}
		fmt.Printf("Background snapshot copies in progress: %d", num)
		if num >= MAX_SNAPSHOT_COPIES {
			fmt.Printf("\n")
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
		fmt.Printf(" ok\n")

		//copy snapshot
		InputName := getNameTag(InputSnapshot.Tags)
		name := strings.ReplaceAll(InputName, "shared-", "received-")
		copyres, err := svc.CopySnapshot(&ec2.CopySnapshotInput{
			Description:      InputSnapshot.Description,
			KmsKeyId:         aws.String(KMSArn),
			Encrypted:        aws.Bool(true),
			SourceRegion:     aws.String("us-gov-west-1"),
			SourceSnapshotId: InputSnapshot.SnapshotId,
			TagSpecifications: []*ec2.TagSpecification{
				{
					ResourceType: aws.String("snapshot"),
					Tags: []*ec2.Tag{
						{
							Key:   aws.String("Name"),
							Value: aws.String(name),
						},
					},
				},
			},
		})
		if err != nil {
			fmt.Println("failed to copy snapshot: ", err)
			continue
		}
		fmt.Printf("Snapshot copy initiated: %s -> %s \n", aws.StringValue(InputSnapshot.SnapshotId), aws.StringValue(copyres.SnapshotId))
		fmt.Println()
	}
	fmt.Println("Complete")
}
