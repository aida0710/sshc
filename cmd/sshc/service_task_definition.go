package main

import (
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"
	"unicode/utf16"
)

const (
	// serviceTaskNamePrefix starts the per-user task name. Task Scheduler keeps one root
	// folder per machine, so per-user software appends the user's SID, as OneDrive does
	// ("OneDrive Startup Task-S-1-5-21-…"). A SID, unlike a user name, never changes.
	serviceTaskNamePrefix = "sshc Startup Task-"
	serviceTaskMarker     = "Managed by sshc: starts the sshc engine at logon."
	serviceTaskNamespace  = "http://schemas.microsoft.com/windows/2004/02/mit/task"
	// Give transient startup failures time to clear, but stop retrying persistent failures.
	serviceTaskRestartInterval = "PT1M"
	serviceTaskRestartCount    = 3
)

// taskElement retains unknown settings, so a changed trigger/action is never silently
// accepted as the current definition. The scheduler can reorder elements and add defaults.
type taskElement struct {
	XMLName    xml.Name
	Attributes []xml.Attr    `xml:",any,attr"`
	Text       string        `xml:",chardata"`
	Children   []taskElement `xml:",any"`
}

func (element taskElement) at(names ...string) taskElement {
	for _, name := range names {
		var matches []taskElement
		for _, child := range element.Children {
			if child.XMLName.Local == name {
				matches = append(matches, child)
			}
		}
		if len(matches) != 1 {
			return taskElement{}
		}
		element = matches[0]
	}
	return element
}

func parseServiceTask(contents string) (taskElement, error) {
	// COM returns Unicode text, while its declaration still says UTF-16.
	contents = strings.TrimSpace(contents)
	if strings.HasPrefix(contents, "<?xml") {
		_, contents, _ = strings.Cut(contents, "?>")
	}
	var task taskElement
	if err := xml.Unmarshal([]byte(contents), &task); err != nil {
		return task, fmt.Errorf("read scheduled task XML: %w", err)
	}
	if task.XMLName.Local != "Task" || task.XMLName.Space != serviceTaskNamespace {
		return task, errors.New("invalid scheduled task XML namespace")
	}
	return task, nil
}

// serviceTaskName is the name of the logon task of the user with this SID.
func serviceTaskName(sid string) string { return serviceTaskNamePrefix + sid }

func taskExecutablePath(executable string) string { return strings.ReplaceAll(executable, `\`, "/") }
func validTaskExecutable(executable string) bool {
	normalized := taskExecutablePath(executable)
	// A drive-qualified or UNC path is required; never let the scheduler search PATH.
	absolute := len(normalized) > 3 && normalized[1] == ':' && normalized[2] == '/' || strings.HasPrefix(normalized, "//")
	return absolute && !containsControl(executable) && !strings.ContainsAny(executable, `"<>|?*`) && strings.EqualFold(path.Base(normalized), "sshc.exe")
}
func sameTaskExecutable(first, second string) bool {
	return strings.EqualFold(path.Clean(taskExecutablePath(first)), path.Clean(taskExecutablePath(second)))
}
func (task taskElement) executable() string { return task.at("Actions", "Exec", "Command").Text }
func (task taskElement) managedBy(sid string) bool {
	actions := task.at("Actions")
	return task.at("RegistrationInfo", "Description").Text == serviceTaskMarker &&
		len(actions.Children) == 1 && validTaskExecutable(task.executable()) &&
		task.at("Principals", "Principal", "UserId").Text == sid
}

func serviceTaskXML(executable, sid string) (string, error) {
	if !validTaskExecutable(executable) {
		return "", errors.New("service executable must be an absolute path to sshc.exe")
	}
	if !strings.HasPrefix(sid, "S-1-") || containsControl(sid) {
		return "", errors.New("invalid current user SID")
	}
	escape := func(value string) string {
		var output bytes.Buffer
		_ = xml.EscapeText(&output, []byte(value))
		return output.String()
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="%s">
 <RegistrationInfo><Description>%s</Description></RegistrationInfo>
 <Triggers><LogonTrigger><Enabled>true</Enabled><UserId>%s</UserId></LogonTrigger></Triggers>
 <Principals><Principal id="Author"><UserId>%s</UserId><LogonType>InteractiveToken</LogonType><RunLevel>LeastPrivilege</RunLevel></Principal></Principals>
 <Settings>
  <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
  <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
  <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
  <AllowHardTerminate>false</AllowHardTerminate>
  <StartWhenAvailable>false</StartWhenAvailable>
  <RunOnlyIfNetworkAvailable>false</RunOnlyIfNetworkAvailable>
  <IdleSettings><StopOnIdleEnd>false</StopOnIdleEnd><RestartOnIdle>false</RestartOnIdle></IdleSettings>
  <AllowStartOnDemand>true</AllowStartOnDemand><Enabled>true</Enabled><Hidden>false</Hidden>
  <RunOnlyIfIdle>false</RunOnlyIfIdle><WakeToRun>false</WakeToRun>
  <ExecutionTimeLimit>PT0S</ExecutionTimeLimit><Priority>7</Priority>
  <RestartOnFailure><Interval>%s</Interval><Count>%d</Count></RestartOnFailure>
 </Settings>
 <Actions Context="Author"><Exec><Command>%s</Command><Arguments>engine --background</Arguments></Exec></Actions>
</Task>`, serviceTaskNamespace, serviceTaskMarker, escape(sid), escape(sid), serviceTaskRestartInterval, serviceTaskRestartCount, escape(executable)), nil
}

func taskUTF16(contents string) []byte {
	words := utf16.Encode([]rune(contents))
	output := make([]byte, 2+len(words)*2)
	binary.LittleEndian.PutUint16(output, 0xfeff)
	for index, word := range words {
		binary.LittleEndian.PutUint16(output[2+index*2:], word)
	}
	return output
}

// taskSchemaDefaults are the Task Scheduler 1.2 schema defaults that the scheduler omits
// from the XML it returns. An element with its default value means the same as no element,
// so both sides drop it before comparison.
// https://learn.microsoft.com/en-us/windows/win32/taskschd/task-scheduler-schema-elements
var taskSchemaDefaults = map[string]string{
	"LogonTrigger/Enabled":               "true",
	"Principal/RunLevel":                 "LeastPrivilege",
	"Settings/AllowStartOnDemand":        "true",
	"Settings/Enabled":                   "true",
	"Settings/Hidden":                    "false",
	"Settings/MultipleInstancesPolicy":   "IgnoreNew",
	"Settings/Priority":                  "7",
	"Settings/RunOnlyIfIdle":             "false",
	"Settings/RunOnlyIfNetworkAvailable": "false",
	"Settings/StartWhenAvailable":        "false",
	"Settings/WakeToRun":                 "false",
	"IdleSettings/RestartOnIdle":         "false",
}

// withAccountSIDs returns the task with account names in its trigger and principal
// replaced by SIDs. The scheduler stores the SID it was given for the principal but
// returns the logon trigger's user as an account name.
func (task taskElement) withAccountSIDs(lookup func(string) (string, error)) (taskElement, error) {
	var replace func(element taskElement, parent string) (taskElement, error)
	replace = func(element taskElement, parent string) (taskElement, error) {
		name := element.XMLName.Local
		if name == "UserId" && (parent == "LogonTrigger" || parent == "Principal") {
			account := strings.TrimSpace(element.Text)
			if account != "" && !strings.HasPrefix(account, "S-1-") {
				sid, err := lookup(account)
				if err != nil {
					return element, err
				}
				element.Text = sid
			}
			return element, nil
		}
		children := make([]taskElement, len(element.Children))
		for index, child := range element.Children {
			replaced, err := replace(child, name)
			if err != nil {
				return element, err
			}
			children[index] = replaced
		}
		element.Children = children
		return element, nil
	}
	return replace(task, "")
}

// canonicalTask ignores only scheduler metadata and documented default settings.
// Extra actions, triggers, constraints and changed values remain visible to comparison.
func canonicalTask(element taskElement, parent string) string {
	name := element.XMLName.Local
	if parent == "RegistrationInfo" && (name == "URI" || name == "Date" || name == "Author") {
		return ""
	}
	if parent == "Settings" && element.Text == "false" && (name == "UseUnifiedSchedulingEngine" || name == "DisallowStartOnRemoteAppSession" || name == "Volatile") {
		return ""
	}
	// These optional defaults may be materialized or omitted by Task Scheduler.
	// https://learn.microsoft.com/en-us/windows/win32/taskschd/taskschedulerschema-idlesettingstype-complextype
	if parent == "IdleSettings" && (name == "Duration" && element.Text == "PT10M" || name == "WaitTimeout" && element.Text == "PT1H") {
		return ""
	}
	if parent == "Exec" && name == "WorkingDirectory" && strings.TrimSpace(element.Text) == "" {
		return ""
	}
	if value, ok := taskSchemaDefaults[parent+"/"+name]; ok && strings.TrimSpace(element.Text) == value && len(element.Children) == 0 {
		return ""
	}
	var children []string
	for _, child := range element.Children {
		if canonical := canonicalTask(child, name); canonical != "" {
			children = append(children, canonical)
		}
	}
	slices.Sort(children)
	var attributes []string
	for _, attribute := range element.Attributes {
		if attribute.Name.Local == "xmlns" || name == "Task" && attribute.Name.Local == "version" || name == "Principal" && attribute.Name.Local == "id" || name == "Actions" && attribute.Name.Local == "Context" {
			continue
		}
		attributes = append(attributes, fmt.Sprintf("%s=%q", attribute.Name.Local, attribute.Value))
	}
	slices.Sort(attributes)
	value := strings.TrimSpace(element.Text)
	if name == "Command" {
		value = strings.ToLower(path.Clean(taskExecutablePath(value)))
	}
	return fmt.Sprintf("%s[%s]%q{%s}", name, strings.Join(attributes, ","), value, strings.Join(children, ""))
}

func (task taskElement) matches(executable, sid string) bool {
	expectedXML, err := serviceTaskXML(executable, sid)
	if err != nil {
		return false
	}
	expected, err := parseServiceTask(expectedXML)
	return err == nil && canonicalTask(task, "") == canonicalTask(expected, "")
}
