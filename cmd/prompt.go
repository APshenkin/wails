package cmd

import (
	"fmt"
	"strconv"
)

// Prompt asks the user for a value
func Prompt(question string, defaultValue ...string) string {
	var answer string
	haveDefault := len(defaultValue) > 0 && defaultValue[0] != ""

	if haveDefault {
		question = fmt.Sprintf("%s (%s)", question, defaultValue[0])
	}
	fmt.Printf(question + ": ")
	fmt.Scanln(&answer)
	if haveDefault {
		if len(answer) == 0 {
			answer = defaultValue[0]
		}
	}
	return answer
}

// PromptRequired calls Prompt repeatedly until a value is given
func PromptRequired(question string, defaultValue ...string) string {
	for {
		result := Prompt(question, defaultValue...)
		if result != "" {
			return result
		}
	}
}

// PromptSelection asks the user to choose an option
func PromptSelection(question string, options []string, optionalDefaultValue ...int) int {

	defaultValue := -1
	message := "Please choose an option"
	fmt.Println(question + ":")

	if len(optionalDefaultValue) > 0 {
		defaultValue = optionalDefaultValue[0] + 1
		message = fmt.Sprintf("%s [%d]", message, defaultValue)
	}

	for index, option := range options {
		fmt.Printf("  %d: %s\n", index+1, option)
	}

	selectedValue := -1

	for {
		choice := Prompt(message)
		if choice == "" && defaultValue > -1 {
			selectedValue = defaultValue - 1
			break
		}

		// index
		number, err := strconv.Atoi(choice)
		if err == nil {
			if number > 0 && number <= len(options) {
				selectedValue = number - 1
				break
			} else {
				continue
			}
		}

	}

	return selectedValue
}
